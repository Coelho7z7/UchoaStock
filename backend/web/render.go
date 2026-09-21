package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"uchoastock/frontend"
)

// Os templates e os arquivos estáticos vêm embutidos no binário (pacote
// frontend) e são preparados uma vez só, quando o programa sobe. Antes,
// cada requisição lia e interpretava o HTML do disco de novo, e um erro de
// sintaxe num template só aparecia quando alguém abria aquela tela.
var (
	// assetVersions guarda um pedaço do hash de cada arquivo estático,
	// pelo caminho ("css/pages.css"). Vira o ?v= do endereço e o ETag.
	assetVersions = hashStatic(frontend.Static)

	// pages tem um template pronto por tela, pelo nome do arquivo sem
	// ".html" ("materials"). pagesErr é o erro da carga, se houve.
	pages, pagesErr = loadPages(frontend.Templates)
)

// CheckTemplates devolve o erro da carga dos templates. O main chama na
// inicialização, para o servidor nem subir com um template quebrado.
func CheckTemplates() error {
	return pagesErr
}

// layoutView é o que o layout "app" recebe: os dados da tela e qual item
// do menu fica aceso. A página monta com {{ template "app" (layout . "materiais") }}.
type layoutView struct {
	Page   any
	Active string
}

var templateFuncs = template.FuncMap{
	"layout": func(page any, active string) layoutView {
		return layoutView{Page: page, Active: active}
	},
	"asset": assetURL,
}

// assetURL devolve o endereço de um arquivo estático com a versão:
// "css/pages.css" vira "/css/pages.css?v=1a2b3c4d5e". Arquivo que não
// existe é erro, para um nome digitado errado não passar em silêncio.
func assetURL(name string) (string, error) {
	version, ok := assetVersions[name]
	if !ok {
		return "", fmt.Errorf("arquivo estático não encontrado: %s", name)
	}
	return "/" + name + "?v=" + version, nil
}

// loadPages interpreta os templates: layouts/ e partials/ formam a base,
// e cada arquivo de pages/ ganha uma cópia da base com ele por cima. A
// cópia é por tela porque todas definem os mesmos blocos ("title",
// "content"...): juntas, uma apagaria a outra.
func loadPages(files fs.FS) (map[string]*template.Template, error) {
	base, err := template.New("base").Funcs(templateFuncs).ParseFS(files, "layouts/*.html", "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("templates base: %w", err)
	}

	names, err := fs.Glob(files, "pages/*.html")
	if err != nil {
		return nil, err
	}

	loaded := make(map[string]*template.Template, len(names))
	for _, name := range names {
		page, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := page.ParseFS(files, name); err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		loaded[strings.TrimSuffix(path.Base(name), ".html")] = page
	}
	return loaded, nil
}

// render monta a tela page (o nome do arquivo em pages/, sem ".html") e
// responde com o status pedido. Monta num buffer antes: se o template
// falhar no meio, a pessoa vê o erro, e não meia página.
func render(w http.ResponseWriter, status int, page string, data any) {
	tmpl, ok := pages[page]
	if !ok {
		log.Printf("template %q não carregado (erro na carga: %v)", page, pagesErr)
		http.Error(w, "Erro ao carregar a página", http.StatusInternalServerError)
		return
	}

	var content bytes.Buffer
	if err := tmpl.ExecuteTemplate(&content, page+".html", data); err != nil {
		log.Printf("erro ao renderizar %s: %v", page, err)
		http.Error(w, "Erro ao renderizar a página", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = content.WriteTo(w)
}

// successURL monta o endereço de volta depois de uma ação que deu certo:
// ?sucesso= escolhe a mensagem, e &destaque= (quando highlightID > 0) é a
// linha que o app.js acende na tabela, para a pessoa achar o que mudou.
func successURL(path, success string, highlightID int) string {
	query := url.Values{"sucesso": {success}}
	if highlightID > 0 {
		query.Set("destaque", strconv.Itoa(highlightID))
	}
	return path + "?" + query.Encode()
}

// formStatus é o status de uma tela com formulário: 400 quando o envio
// deu errado (errorMessage preenchido), 200 no resto.
func formStatus(errorMessage string) int {
	if errorMessage != "" {
		return http.StatusBadRequest
	}
	return http.StatusOK
}

// hashStatic calcula a versão de cada arquivo estático: os 10 primeiros
// caracteres do SHA-256 do conteúdo.
func hashStatic(files fs.FS) map[string]string {
	versions := map[string]string{}
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		versions[name] = hex.EncodeToString(sum[:])[:10]
		return nil
	})
	if err != nil {
		log.Println("erro ao ler os arquivos estáticos:", err)
	}
	return versions
}

// staticHandler serve CSS, JS, fontes e imagens. O conteúdo embutido não
// tem data de modificação, então quem diz ao navegador se o arquivo mudou
// é o ETag (o hash). Pedido com o ?v= certo pode ficar em cache por um
// ano: quando o arquivo mudar, o endereço muda junto. Sem o ?v= (fontes,
// imagens), o navegador confere o ETag a cada uso e recebe 304 se nada
// mudou. Pasta não é listada.
func staticHandler() http.Handler {
	files := http.FileServerFS(frontend.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		version, ok := assetVersions[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"`+version+`"`)
		if r.URL.Query().Get("v") == version {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
