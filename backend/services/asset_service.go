package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	database "uchoastock/backend/database"
)

// Tamanho máximo, em letras, de cada campo do bem.
const (
	maxAssetNumberLength = 30
	maxAssetNameLength   = 100
	maxAssetTextLength   = 300
)

// AssetInputError é um erro causado pelo que a pessoa digitou ou pediu
// (número repetido, obra concluída...). A mensagem pode ir para a tela; a
// de um erro do banco não, porque revelaria detalhe interno.
type AssetInputError struct {
	Message string
}

func (e AssetInputError) Error() string {
	return e.Message
}

// ErrAssetNotFound é devolvido quando o bem não existe.
var ErrAssetNotFound = AssetInputError{"bem não encontrado"}

// ErrAssetMoved é devolvido quando o bem saiu da obra que estava na tela
// (outra pessoa transferiu enquanto a página estava aberta). A permissão
// foi conferida para a obra antiga, então a ação não pode seguir.
var ErrAssetMoved = AssetInputError{"este bem mudou de obra enquanto a tela estava aberta. Confira e tente de novo"}

// ErrReactivateNotAllowed é devolvido quando alguém sem permissão tenta
// tirar um bem de baixado. Não é AssetInputError de propósito: o handler
// responde "Acesso negado".
var ErrReactivateNotAllowed = errors.New("reativar bem baixado é só para administrador")

// assetStatusTransitions diz para quais situações um bem pode ir a partir
// de cada uma. Baixado só volta para em uso (reativar), e só com
// permissão.
var assetStatusTransitions = map[string][]string{
	AssetStatusInUse:       {AssetStatusMaintenance, AssetStatusRetired},
	AssetStatusMaintenance: {AssetStatusInUse, AssetStatusRetired},
	AssetStatusRetired:     {AssetStatusInUse},
}

func canTransitionAsset(from, to string) bool {
	for _, allowed := range assetStatusTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// NextAssetStatuses devolve as situações para as quais o bem pode ir a
// partir de current, na ordem de AssetStatusLabels, para a tela montar o
// select. Sem allowReactivate, bem baixado não tem para onde ir.
func NextAssetStatuses(current string, allowReactivate bool) []struct{ Value, Label string } {
	var next []struct{ Value, Label string }
	if current == AssetStatusRetired && !allowReactivate {
		return next
	}
	for _, s := range AssetStatusLabels {
		if canTransitionAsset(current, s.Value) {
			next = append(next, s)
		}
	}
	return next
}

// normalizeAssetFields tira os espaços das pontas e valida número, nome e
// descrição. O número fica em maiúsculas: "pat-001" e "PAT-001" são a
// mesma plaqueta. strings.ToUpper do Go entende acento, ao contrário do
// UPPER do SQLite.
func normalizeAssetFields(number, name, description string) (string, string, string, error) {
	number = strings.ToUpper(strings.TrimSpace(number))
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)

	switch {
	case number == "":
		return "", "", "", AssetInputError{"informe o número de patrimônio"}
	case utf8.RuneCountInString(number) > maxAssetNumberLength:
		return "", "", "", AssetInputError{fmt.Sprintf("o número de patrimônio pode ter no máximo %d caracteres", maxAssetNumberLength)}
	case name == "":
		return "", "", "", AssetInputError{"informe o nome do bem"}
	case utf8.RuneCountInString(name) > maxAssetNameLength:
		return "", "", "", AssetInputError{fmt.Sprintf("o nome do bem pode ter no máximo %d caracteres", maxAssetNameLength)}
	case utf8.RuneCountInString(description) > maxAssetTextLength:
		return "", "", "", AssetInputError{fmt.Sprintf("a descrição pode ter no máximo %d caracteres", maxAssetTextLength)}
	}
	return number, name, description, nil
}

// normalizeAssetNote valida a observação de transferência e de mudança
// de situação.
func normalizeAssetNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxAssetTextLength {
		return "", AssetInputError{fmt.Sprintf("a observação pode ter no máximo %d caracteres", maxAssetTextLength)}
	}
	return note, nil
}

// CreateAsset cadastra um bem novo, em uso, na obra siteID, e registra o
// cadastro no histórico. Devolve o ID do bem.
func CreateAsset(userID int, number, name, description string, siteID int) (int, error) {
	number, name, description, err := normalizeAssetFields(number, name, description)
	if err != nil {
		return 0, err
	}

	// A checagem do número e o INSERT ficam na mesma transação: com uma
	// conexão só (SetMaxOpenConns(1)), dois cadastros simultâneos do mesmo
	// número não passam os dois. A coluna ainda é UNIQUE, por garantia.
	tx, err := database.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if err := checkAssetDestination(tx, siteID); err != nil {
		return 0, err
	}
	if err := checkAssetNumberFree(tx, number, 0); err != nil {
		return 0, err
	}

	result, err := tx.Exec(`
		INSERT INTO patrimonios (numero, nome, descricao, obra_id, situacao)
		VALUES (?, ?, ?, ?, ?)
	`, number, name, description, siteID, AssetStatusInUse)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(`
		INSERT INTO patrimonio_movimentacoes (patrimonio_id, usuario_id, acao, obra_destino_id, situacao)
		VALUES (?, ?, ?, ?, ?)
	`, id, userID, AssetEventCreated, siteID, AssetStatusInUse); err != nil {
		return 0, err
	}

	return int(id), tx.Commit()
}

// UpdateAsset altera número, nome e descrição do bem. O que mudou vai
// para o histórico ("Nome: Furadeira → Furadeira de impacto"); salvar sem
// mudar nada não gera linha no histórico.
func UpdateAsset(userID, id int, number, name, description string) error {
	number, name, description, err := normalizeAssetFields(number, name, description)
	if err != nil {
		return err
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldNumber, oldName, oldDescription string
	err = tx.QueryRow(`SELECT numero, nome, descricao FROM patrimonios WHERE id = ?`, id).
		Scan(&oldNumber, &oldName, &oldDescription)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAssetNotFound
	}
	if err != nil {
		return err
	}

	if err := checkAssetNumberFree(tx, number, id); err != nil {
		return err
	}

	var changes []string
	if number != oldNumber {
		changes = append(changes, fmt.Sprintf("Número: %s → %s", oldNumber, number))
	}
	if name != oldName {
		changes = append(changes, fmt.Sprintf("Nome: %s → %s", oldName, name))
	}
	if description != oldDescription {
		changes = append(changes, "Descrição alterada")
	}
	if len(changes) == 0 {
		return nil
	}

	if _, err := tx.Exec(`
		UPDATE patrimonios SET numero = ?, nome = ?, descricao = ? WHERE id = ?
	`, number, name, description, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO patrimonio_movimentacoes (patrimonio_id, usuario_id, acao, observacao)
		VALUES (?, ?, ?, ?)
	`, id, userID, AssetEventEdit, strings.Join(changes, "; ")); err != nil {
		return err
	}

	return tx.Commit()
}

// TransferAsset leva o bem da obra fromSiteID para a obra toSiteID.
// fromSiteID é a obra em que o handler conferiu a permissão: se o bem já
// não está mais nela, a transferência é recusada (ErrAssetMoved).
//
// Bem baixado não é transferido. O destino não pode estar concluído; a
// origem pode, porque é justamente de obra encerrada que a ferramenta
// precisa sair.
func TransferAsset(userID, id, fromSiteID, toSiteID int, note string) error {
	note, err := normalizeAssetNote(note)
	if err != nil {
		return err
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	currentSite, status, err := lockAsset(tx, id, fromSiteID)
	if err != nil {
		return err
	}
	if status == AssetStatusRetired {
		return AssetInputError{"bem baixado não pode ser transferido"}
	}
	if toSiteID == currentSite {
		return AssetInputError{"o bem já está nesta obra"}
	}
	if err := checkAssetDestination(tx, toSiteID); err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE patrimonios SET obra_id = ? WHERE id = ?`, toSiteID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO patrimonio_movimentacoes (patrimonio_id, usuario_id, acao, obra_origem_id, obra_destino_id, observacao)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, userID, AssetEventTransfer, currentSite, toSiteID, note); err != nil {
		return err
	}

	return tx.Commit()
}

// ChangeAssetStatus muda a situação do bem: mandar para manutenção, voltar
// a usar, dar baixa ou reativar. fromSiteID funciona como em
// TransferAsset. Dar baixa exige o motivo (na observação); reativar exige
// allowReactivate.
func ChangeAssetStatus(userID, id, fromSiteID int, status, note string, allowReactivate bool) error {
	if !isValidAssetStatus(status) {
		return AssetInputError{"situação inválida"}
	}
	note, err := normalizeAssetNote(note)
	if err != nil {
		return err
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, current, err := lockAsset(tx, id, fromSiteID)
	if err != nil {
		return err
	}
	if current == status {
		return AssetInputError{"o bem já está " + strings.ToLower(assetStatusLabel(status))}
	}
	if !canTransitionAsset(current, status) {
		return AssetInputError{fmt.Sprintf("um bem %s não pode passar para %s",
			strings.ToLower(assetStatusLabel(current)), strings.ToLower(assetStatusLabel(status)))}
	}
	if current == AssetStatusRetired && !allowReactivate {
		return ErrReactivateNotAllowed
	}
	if status == AssetStatusRetired && note == "" {
		return AssetInputError{"informe o motivo da baixa na observação"}
	}

	if _, err := tx.Exec(`UPDATE patrimonios SET situacao = ? WHERE id = ?`, status, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO patrimonio_movimentacoes (patrimonio_id, usuario_id, acao, situacao, observacao)
		VALUES (?, ?, ?, ?, ?)
	`, id, userID, AssetEventStatus, status, note); err != nil {
		return err
	}

	return tx.Commit()
}

// lockAsset lê a obra e a situação do bem dentro da transação e confere
// que ele ainda está na obra fromSiteID.
func lockAsset(tx *sql.Tx, id, fromSiteID int) (int, string, error) {
	var siteID int
	var status string
	err := tx.QueryRow(`SELECT obra_id, situacao FROM patrimonios WHERE id = ?`, id).Scan(&siteID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrAssetNotFound
	}
	if err != nil {
		return 0, "", err
	}
	if siteID != fromSiteID {
		return 0, "", ErrAssetMoved
	}
	return siteID, status, nil
}

// checkAssetDestination confere se a obra existe e ainda recebe bens
// (não está concluída).
func checkAssetDestination(tx *sql.Tx, siteID int) error {
	var name, status string
	err := tx.QueryRow(`SELECT nome, situacao FROM obras WHERE id = ? AND ativo = 1`, siteID).Scan(&name, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return AssetInputError{"escolha a obra"}
	}
	if err != nil {
		return err
	}
	if status == SiteStatusFinished {
		return AssetInputError{fmt.Sprintf("a obra %s está concluída e não recebe bens", name)}
	}
	return nil
}

// checkAssetNumberFree recusa um número que já é de outro bem, inclusive
// de um baixado: a plaqueta é única para sempre. exceptID é o próprio bem,
// na edição (0 no cadastro).
func checkAssetNumberFree(tx *sql.Tx, number string, exceptID int) error {
	var count int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM patrimonios WHERE numero = ? AND id <> ?
	`, number, exceptID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return AssetInputError{"já existe um bem com o número " + number}
	}
	return nil
}
