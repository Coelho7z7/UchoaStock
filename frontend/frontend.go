// Package frontend embute no binário os templates e os arquivos estáticos
// (CSS, JS, fontes e imagens). Com isso o servidor não depende da pasta de
// onde é chamado: o executável sozinho já tem tudo o que a tela precisa.
//
// Mudou um HTML, CSS ou JS? Reinicie o servidor (go run ./backend/cmd):
// o conteúdo embutido é o da hora da compilação.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed templates static
var files embed.FS

// Templates são os templates HTML: layouts/, partials/ e pages/.
var Templates = mustSub("templates")

// Static são os arquivos servidos direto ao navegador: css/, js/, images/.
var Static = mustSub("static")

func mustSub(dir string) fs.FS {
	sub, err := fs.Sub(files, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
