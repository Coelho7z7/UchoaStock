package models

// Asset é um bem do patrimônio: uma ferramenta ou equipamento com número
// próprio (a plaqueta), que está numa obra só. Status guarda o valor do
// banco ("EM_USO"); FormattedStatus, o texto da tela ("Em uso").
type Asset struct {
	ID              int
	Number          string
	Name            string
	Description     string
	SiteID          int
	SiteName        string
	Status          string
	FormattedStatus string
	CreatedAt       string
}

// AssetEvent é uma linha do histórico do bem: cadastro, transferência,
// mudança de situação ou edição dos dados.
type AssetEvent struct {
	Action          string
	FormattedAction string
	UserName        string
	// FromSiteName e ToSiteName só vêm na transferência (no cadastro, só
	// ToSiteName).
	FromSiteName string
	ToSiteName   string
	// Status é a situação nova, só quando ela mudou.
	Status          string
	FormattedStatus string
	Note            string
	CreatedAt       string
}
