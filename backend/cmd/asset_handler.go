package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"uchoastock/backend/models"
	"uchoastock/backend/services"
)

// assetForm guarda o que foi digitado nos formulários do patrimônio, para
// a tela voltar preenchida quando o envio dá erro. Action diz de qual
// formulário veio o envio ("transferir", "situacao" ou "editar"): só ele
// volta preenchido.
type assetForm struct {
	Action      string
	Number      string
	Name        string
	Description string
	SiteID      int
	Status      string
	Note        string
}

// assetListHandler mostra a lista de bens da obra do seletor (/patrimonio)
// e processa o cadastro (POST). Com ?novo=1 a tela já abre com o modal de
// cadastro, para funcionar também sem JS.
func assetListHandler(w http.ResponseWriter, r *http.Request, user *models.User) {
	scope, ok := requireSiteScope(w, r, user)
	if !ok {
		return
	}

	data := struct {
		User           *models.User
		Scope          siteScope
		Nav            navData
		CanManageUsers bool
		// CanCreate mostra o botão "Novo bem" (só administrador).
		CanCreate bool
		Statuses  []struct{ Value, Label string }
		StatusAll string
		Status    string
		Search    string
		Assets    []models.Asset
		// Form != nil abre o modal de cadastro.
		Form      *assetForm
		FormError string
	}{
		User:           user,
		Scope:          scope,
		Nav:            buildNav(user, scope),
		CanManageUsers: can(user, PermManageUsers),
		CanCreate:      can(user, PermEditAsset),
		Statuses:       services.AssetStatusLabels,
		StatusAll:      services.AssetStatusAll,
		Status:         r.URL.Query().Get("situacao"),
		Search:         strings.TrimSpace(r.URL.Query().Get("busca")),
	}

	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("novo") != "" && data.CanCreate {
			// O modal já vem com a obra do seletor escolhida.
			data.Form = &assetForm{SiteID: scope.SiteID()}
		}
	case http.MethodPost:
		if r.FormValue("acao") != "cadastrar" {
			http.Error(w, "Ação inválida", http.StatusBadRequest)
			return
		}
		if !requirePermission(w, user, PermEditAsset) {
			return
		}
		form := &assetForm{
			Number:      r.FormValue("numero"),
			Name:        r.FormValue("nome"),
			Description: r.FormValue("descricao"),
		}
		form.SiteID, _ = strconv.Atoi(r.FormValue("obra"))

		id, err := services.CreateAsset(user.ID, form.Number, form.Name, form.Description, form.SiteID)
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/patrimonio/%d?sucesso=cadastrado", id), http.StatusSeeOther)
			return
		}
		data.Form = form
		data.FormError = assetErrorMessage(err)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	assets, err := services.GetAssets(scope.SiteID(), data.Search, data.Status)
	if err != nil {
		log.Println("erro em GetAssets:", err)
		http.Error(w, "Erro ao buscar o patrimônio", http.StatusInternalServerError)
		return
	}
	data.Assets = assets

	status := http.StatusOK
	if data.FormError != "" {
		status = http.StatusBadRequest
	}
	renderPage(w, status, "frontend/html/assets.html", data)
}

// assetDetailHandler mostra um bem (/patrimonio/{id}) com o histórico e
// processa as ações: transferir, mudar a situação e editar os dados.
func assetDetailHandler(w http.ResponseWriter, r *http.Request, user *models.User) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		renderNotFound(w, r, user)
		return
	}
	scope, ok := requireSiteScope(w, r, user)
	if !ok {
		return
	}

	asset, err := services.GetAssetByID(id)
	if errors.Is(err, services.ErrAssetNotFound) {
		renderNotFound(w, r, user)
		return
	}
	if err != nil {
		log.Println("erro em GetAssetByID:", err)
		http.Error(w, "Erro ao buscar o bem", http.StatusInternalServerError)
		return
	}

	data := struct {
		User           *models.User
		Scope          siteScope
		Nav            navData
		CanManageUsers bool
		Asset          *models.Asset
		Events         []models.AssetEvent
		// CanMove: transferir e mudar a situação, só para quem tem
		// PermMoveAsset na obra em que o bem está (ver canMoveAssetAt).
		CanMove bool
		// MoveElsewhere explica por que os botões sumiram para quem
		// movimenta bens, mas não na obra deste.
		MoveElsewhere bool
		CanEdit       bool
		NextStatuses  []struct{ Value, Label string }
		Message       string
		Error         string
		Form          assetForm
	}{
		User:           user,
		Scope:          scope,
		Nav:            buildNav(user, scope),
		CanManageUsers: can(user, PermManageUsers),
		CanMove:        canMoveAssetAt(user, asset.SiteID),
		CanEdit:        can(user, PermEditAsset),
	}
	data.MoveElsewhere = can(user, PermMoveAsset) && !data.CanMove

	data.Message = map[string]string{
		"cadastrado":  "Bem cadastrado.",
		"transferido": "Bem transferido.",
		"situacao":    "Situação do bem alterada.",
		"atualizado":  "Dados do bem salvos.",
	}[r.URL.Query().Get("sucesso")]

	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		form := assetForm{
			Action:      r.FormValue("acao"),
			Number:      r.FormValue("numero"),
			Name:        r.FormValue("nome"),
			Description: r.FormValue("descricao"),
			Status:      r.FormValue("situacao"),
			Note:        r.FormValue("observacao"),
		}
		form.SiteID, _ = strconv.Atoi(r.FormValue("obra_destino"))

		// A obra em que o bem estava quando a tela foi montada. Se ele
		// mudou de obra desde então, a pessoa age sobre um dado velho.
		shownSiteID, _ := strconv.Atoi(r.FormValue("obra_atual"))

		success := ""
		switch form.Action {
		case "transferir":
			if !data.CanMove {
				renderAccessDenied(w)
				return
			}
			err = services.ErrAssetMoved
			if shownSiteID == asset.SiteID {
				err = services.TransferAsset(user.ID, asset.ID, asset.SiteID, form.SiteID, form.Note)
			}
			success = "transferido"
		case "situacao":
			if !data.CanMove {
				renderAccessDenied(w)
				return
			}
			err = services.ErrAssetMoved
			if shownSiteID == asset.SiteID {
				err = services.ChangeAssetStatus(user.ID, asset.ID, asset.SiteID, form.Status, form.Note, can(user, PermEditAsset))
			}
			success = "situacao"
		case "editar":
			if !requirePermission(w, user, PermEditAsset) {
				return
			}
			err = services.UpdateAsset(user.ID, asset.ID, form.Number, form.Name, form.Description)
			success = "atualizado"
		default:
			http.Error(w, "Ação inválida", http.StatusBadRequest)
			return
		}

		if errors.Is(err, services.ErrReactivateNotAllowed) {
			renderAccessDenied(w)
			return
		}
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/patrimonio/%d?sucesso=%s", asset.ID, success), http.StatusSeeOther)
			return
		}
		data.Error = assetErrorMessage(err)
		data.Form = form
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// O formulário de edição começa com os dados atuais; depois de um
	// erro nele, com o que foi digitado.
	if data.Form.Action != "editar" {
		data.Form.Number = asset.Number
		data.Form.Name = asset.Name
		data.Form.Description = asset.Description
	}

	data.Asset = asset
	data.NextStatuses = services.NextAssetStatuses(asset.Status, can(user, PermEditAsset))
	if data.Events, err = services.GetAssetEvents(asset.ID); err != nil {
		log.Println("erro em GetAssetEvents:", err)
		http.Error(w, "Erro ao buscar o histórico do bem", http.StatusInternalServerError)
		return
	}

	status := http.StatusOK
	if data.Error != "" {
		status = http.StatusBadRequest
	}
	renderPage(w, status, "frontend/html/asset_detail.html", data)
}

// assetErrorMessage escolhe o que mostrar na tela. Erro de digitação
// (AssetInputError) aparece como veio; erro do banco vai para o log e a
// pessoa vê uma mensagem genérica, sem detalhe interno.
func assetErrorMessage(err error) string {
	var inputErr services.AssetInputError
	if errors.As(err, &inputErr) {
		return inputErr.Message
	}
	log.Println("erro ao salvar bem:", err)
	return "Não foi possível salvar o bem. Tente novamente."
}
