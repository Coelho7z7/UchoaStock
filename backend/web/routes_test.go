package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Todo template carrega sem erro e toda tela usada pelos handlers existe.
// Um erro de sintaxe num HTML quebra este teste, e não a tela em produção.
func TestTemplatesLoad(t *testing.T) {
	if err := CheckTemplates(); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{
		"access_denied", "asset_detail", "assets", "dashboard", "edit_material",
		"inventories", "inventory_detail", "login", "materials", "movements",
		"my_password", "not_found", "reports", "request_detail", "request_new",
		"requests", "sites", "stock", "suppliers", "users",
	} {
		if _, ok := pages[page]; !ok {
			t.Errorf("tela %q não carregada", page)
		}
	}
}

// O login e o acesso negado não dependem do banco: dá para renderizar
// direto e conferir que o CSS sai com a versão do arquivo.
func TestRenderUsesVersionedAssets(t *testing.T) {
	recorder := httptest.NewRecorder()
	renderAccessDenied(recorder)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status %d, esperado 403", recorder.Code)
	}
	body := recorder.Body.String()
	want := `href="/css/pages.css?v=` + assetVersions["css/pages.css"] + `"`
	if !strings.Contains(body, want) {
		t.Fatalf("página sem %s", want)
	}
}

// POST vindo de outro site é barrado antes de chegar em qualquer handler;
// do próprio site (ou sem os cabeçalhos, como um script) passa.
func TestCrossSitePostIsBlocked(t *testing.T) {
	handler := NewHandler()
	cases := []struct {
		name    string
		header  string
		value   string
		blocked bool
	}{
		{"outro site", "Sec-Fetch-Site", "cross-site", true},
		{"Origin de outro domínio", "Origin", "https://golpe.example", true},
		{"mesmo site", "Sec-Fetch-Site", "same-origin", false},
		{"sem cabeçalho", "", "", false},
	}
	for _, c := range cases {
		// Arquivo estático: não precisa de banco, e qualquer resposta que
		// não seja o 403 prova que o pedido passou pela proteção.
		request := httptest.NewRequest(http.MethodPost, "/css/tokens.css", nil)
		if c.header != "" {
			request.Header.Set(c.header, c.value)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if blocked := recorder.Code == http.StatusForbidden; blocked != c.blocked {
			t.Errorf("%s: status %d, bloqueado = %v, esperado %v", c.name, recorder.Code, blocked, c.blocked)
		}
	}
}

// Com o ?v= certo o arquivo fica em cache; sem ele, o navegador confere o
// ETag e recebe 304 quando nada mudou. Pasta não é listada.
func TestStaticFilesCache(t *testing.T) {
	handler := NewHandler()
	version := assetVersions["css/tokens.css"]
	if version == "" {
		t.Fatal("css/tokens.css sem versão")
	}

	get := func(path, etag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if etag != "" {
			request.Header.Set("If-None-Match", etag)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	versioned := get("/css/tokens.css?v="+version, "")
	if versioned.Code != http.StatusOK || !strings.Contains(versioned.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("com ?v=: status %d, Cache-Control %q", versioned.Code, versioned.Header().Get("Cache-Control"))
	}
	if got := get("/css/tokens.css", `"`+version+`"`).Code; got != http.StatusNotModified {
		t.Errorf("ETag igual: status %d, esperado 304", got)
	}
	if got := get("/css/", "").Code; got != http.StatusNotFound {
		t.Errorf("listagem da pasta: status %d, esperado 404", got)
	}
	if got := get("/css/nao-existe.css", "").Code; got != http.StatusNotFound {
		t.Errorf("arquivo inexistente: status %d, esperado 404", got)
	}
}
