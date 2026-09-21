package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	database "uchoastock/backend/database"
	"uchoastock/backend/services"
	"uchoastock/backend/utils"
)

// verifyStock é o comando verify-stock, que confere a migração para o
// saldo por obra. Devolve o código de saída (0 = tudo certo), para o teste
// conseguir chamar sem encerrar o processo.
//
//	go run ./backend/cmd verify-stock            (só confere; não altera o banco)
//	go run ./backend/cmd verify-stock --migrate  (migra e confere: só numa CÓPIA)
//
// Sem --migrate, o banco precisa já estar migrado. A conferência compara
// produtos.quantidade com a soma dos saldos, o que só pega migração pela
// metade: logo depois de migrar, as duas batem por construção.
//
// Com --migrate num banco ainda não migrado, o comando tira um retrato do
// estoque antes (e salva em CSV ao lado do banco), roda as migrações e
// confere o resultado contra o retrato.
func verifyStock(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("verify-stock", flag.ContinueOnError)
	flags.SetOutput(out)
	migrate := flags.Bool("migrate", false, "roda as migrações antes de conferir (ALTERA o banco: só numa cópia)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	fmt.Fprintln(out, "Banco:", database.Path())
	code := stockChecks(out, *migrate)

	// A conferência de texto só lê e roda em qualquer caso, inclusive
	// quando o estoque acusou problema: um não esconde o outro.
	if textCode := reportInvalidTexts(out); textCode > code {
		code = textCode
	}
	return code
}

// stockChecks faz as conferências de estoque e de chave estrangeira do
// verify-stock (com ou sem --migrate) e devolve o código de saída.
func stockChecks(out io.Writer, migrate bool) int {

	migrated, err := database.StockMigrated()
	if err != nil {
		fmt.Fprintln(out, "Erro ao ler o banco:", err)
		return 1
	}

	if !migrate {
		if !migrated {
			fmt.Fprintln(out, "ERRO: este banco ainda não foi migrado para o saldo por obra (falta a tabela saldos ou a coluna movimentacoes.obra_id).")
			fmt.Fprintln(out, "Nada foi alterado. Para migrar uma CÓPIA e conferir o resultado, rode de novo com --migrate.")
			return 1
		}
		return reportStockComparison(out)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Fprintln(out, "!!  ATENÇÃO: --migrate ALTERA O BANCO ACIMA.")
	fmt.Fprintln(out, "!!  As migrações vão rodar nele agora. Use SOMENTE numa CÓPIA,")
	fmt.Fprintln(out, "!!  nunca no banco de produção.")
	fmt.Fprintln(out, "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Fprintln(out, "")

	if migrated {
		fmt.Fprintln(out, "O banco já estava migrado: não há como tirar o retrato de antes. Rodando só as migrações pendentes e a conferência simples.")
		if err := database.CreateTables(); err != nil {
			fmt.Fprintln(out, "Erro nas migrações (nada foi gravado):", err)
			return 1
		}
		code := reportStockComparison(out)
		violations, err := database.ForeignKeyViolations()
		if err != nil {
			fmt.Fprintln(out, "Erro ao conferir as chaves estrangeiras:", err)
			return 1
		}
		for _, v := range violations {
			fmt.Fprintln(out, "  PROBLEMA:", foreignKeyProblem(v))
		}
		if len(violations) > 0 {
			fmt.Fprintf(out, "%d chave(s) estrangeira(s) quebrada(s) (PRAGMA foreign_key_check).\n", len(violations))
			return 1
		}
		fmt.Fprintln(out, "Chaves estrangeiras: nenhuma violação (PRAGMA foreign_key_check).")
		if err := reportForeignKeyTables(out); err != nil {
			fmt.Fprintln(out, "Erro ao listar as tabelas conferidas:", err)
			return 1
		}
		return code
	}

	snapshot, err := services.TakeStockSnapshot()
	if err != nil {
		fmt.Fprintln(out, "Erro ao ler o estoque de antes da migração:", err)
		return 1
	}
	csvPath := database.Path() + ".antes-da-migracao.csv"
	if err := writeSnapshotCSV(csvPath, snapshot); err != nil {
		fmt.Fprintln(out, "Erro ao salvar o retrato de antes da migração:", err)
		return 1
	}
	fmt.Fprintf(out, "Retrato de antes da migração: %d material(is), salvo em %s\n", len(snapshot.Materials), csvPath)

	if err := database.CreateTables(); err != nil {
		fmt.Fprintln(out, "Erro na migração (a transação foi desfeita, nada foi gravado):", err)
		return 1
	}
	fmt.Fprintln(out, "Migração concluída. Conferindo contra o retrato...")

	problems, err := services.CheckMigrationAgainstSnapshot(snapshot)
	if err != nil {
		fmt.Fprintln(out, "Erro ao conferir:", err)
		return 1
	}
	violations, err := database.ForeignKeyViolations()
	if err != nil {
		fmt.Fprintln(out, "Erro ao conferir as chaves estrangeiras:", err)
		return 1
	}
	for _, v := range violations {
		problems = append(problems, foreignKeyProblem(v))
	}
	for _, problem := range problems {
		fmt.Fprintln(out, "  PROBLEMA:", problem)
	}
	if len(problems) > 0 {
		fmt.Fprintf(out, "%d problema(s) encontrado(s).\n", len(problems))
		return 1
	}
	fmt.Fprintf(out, "Nenhum problema: %d material(is) com o saldo igual ao de antes, tudo no almoxarifado central, as movimentações ligadas certo e nenhuma chave estrangeira quebrada (PRAGMA foreign_key_check).\n", len(snapshot.Materials))
	if err := reportForeignKeyTables(out); err != nil {
		fmt.Fprintln(out, "Erro ao listar as tabelas conferidas:", err)
		return 1
	}
	return 0
}

// reportForeignKeyTables mostra em quais tabelas o foreign_key_check
// conferiu chave estrangeira. O PRAGMA confere o banco inteiro; a lista
// deixa visível que as tabelas novas (as de solicitação, por exemplo)
// entraram na conta.
func reportForeignKeyTables(out io.Writer) error {
	tables, err := database.TablesWithForeignKeys()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Chaves estrangeiras conferidas em:", strings.Join(tables, ", "))
	return nil
}

// foreignKeyProblem descreve uma chave estrangeira quebrada para a saída
// do verify-stock.
func foreignKeyProblem(v database.ForeignKeyViolation) string {
	return fmt.Sprintf("chave estrangeira quebrada: %s, linha %d, aponta para um registro de %s que não existe", v.Table, v.RowID, v.Parent)
}

// reportStockComparison mostra a conferência simples (produtos.quantidade
// x soma dos saldos) e devolve o código de saída.
func reportStockComparison(out io.Writer) int {
	divergences, checked, err := services.VerifyStockMigration()
	if err != nil {
		fmt.Fprintln(out, "Erro ao conferir o estoque:", err)
		return 1
	}

	fmt.Fprintln(out, "Conferência simples: produtos.quantidade x soma dos saldos das obras")
	fmt.Fprintln(out, "(Só pega banco migrado pela metade ou mexido depois: logo após a migração as duas batem por construção.)")
	for _, d := range divergences {
		status := ""
		if !d.Active {
			status = " [removido]"
		}
		fmt.Fprintf(out, "  #%d %s (%s)%s: produtos.quantidade = %s, soma dos saldos = %s, diferença = %s\n",
			d.MaterialID, d.Name, d.Unit, status,
			utils.FormatQuantity(d.OldQuantity),
			utils.FormatQuantity(d.SiteTotal),
			utils.FormatQuantity(d.SiteTotal-d.OldQuantity))
	}
	fmt.Fprintf(out, "%d material(is) conferido(s), %d divergência(s).\n", checked, len(divergences))

	if len(divergences) > 0 {
		fmt.Fprintln(out, "Num banco já em uso, divergência é esperada: entradas e saídas depois da migração mudam só os saldos.")
		return 1
	}
	return 0
}

// writeSnapshotCSV salva o retrato de antes da migração, para ficar como
// registro junto da cópia do banco. Usa ";" como o CSV das movimentações.
func writeSnapshotCSV(path string, snapshot services.StockSnapshot) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	writer.Comma = ';'
	_ = writer.Write([]string{"id", "nome", "quantidade"})
	for _, m := range snapshot.Materials {
		_ = writer.Write([]string{strconv.Itoa(m.ID), m.Name, strconv.FormatFloat(m.Quantity, 'f', -1, 64)})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// reportInvalidTexts lista os textos de produtos, obras e usuarios que não
// são UTF-8 válido, com o texto provável lido como Latin-1 (a codificação
// do Windows em português). Só lê: a correção é manual, com o texto certo
// (ver INFORMACOES.MD).
func reportInvalidTexts(out io.Writer) int {
	texts, err := database.InvalidUTF8Texts()
	if err != nil {
		fmt.Fprintln(out, "Erro ao conferir a codificação dos textos:", err)
		return 1
	}
	if len(texts) == 0 {
		fmt.Fprintln(out, "Textos: todos em UTF-8 válido (produtos, obras, usuarios).")
		return 0
	}
	for _, t := range texts {
		fmt.Fprintf(out, "  PROBLEMA: texto que não é UTF-8 em %s #%d, coluna %s: %q (lido como Latin-1: %q)\n",
			t.Table, t.RowID, t.Column, string(t.Raw), t.Latin1())
	}
	fmt.Fprintf(out, "%d texto(s) fora de UTF-8. Nada foi alterado: corrija com o texto certo (ver INFORMACOES.MD).\n", len(texts))
	return 1
}
