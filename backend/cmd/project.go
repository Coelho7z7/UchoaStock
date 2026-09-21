package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// prepareProjectDirectory põe o diretório de trabalho na raiz do projeto
// (onde fica o go.mod), independente de onde o programa foi chamado.
// Serve para o caminho padrão do banco (backend/data/uchoastock.db), que
// é relativo. HTML, CSS e JS não dependem mais disso: vêm embutidos no
// binário. Se não achar a raiz (um binário copiado sozinho para outro
// lugar, por exemplo), segue na pasta atual: com DB_PATH absoluto, como
// em produção, nada muda.
func prepareProjectDirectory() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	for {
		if isProjectRoot(dir) {
			_ = os.Chdir(dir)
			return
		}
		// Pasta com o nome do repositório (o git clone cria "UchoaStock"),
		// para rodar a partir da pasta de cima dele.
		if nested := filepath.Join(dir, "UchoaStock"); isProjectRoot(nested) {
			_ = os.Chdir(nested)
			return
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Println("Aviso: raiz do projeto (go.mod) não encontrada; usando a pasta atual.")
			return
		}
		dir = parent
	}
}

// isProjectRoot diz se dir é a raiz do UchôaStock: tem o go.mod e a pasta
// backend/ (o go.mod sozinho poderia ser de outro projeto acima deste).
func isProjectRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "backend"))
	return err == nil && info.IsDir()
}
