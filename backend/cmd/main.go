package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	database "uchoastock/backend/database"
	"uchoastock/backend/services"
	"uchoastock/backend/web"
)

func main() {
	// Antes de tudo: as datas da tela e o "hoje" seguem o horário de
	// Brasília, qualquer que seja o fuso do servidor.
	if err := services.SetupTimezone(); err != nil {
		fmt.Println("Erro ao configurar o fuso horário:", err)
		os.Exit(1)
	}

	prepareProjectDirectory()

	if err := database.Connect(); err != nil {
		fmt.Println("Erro ao conectar ao banco de dados:", err)
		os.Exit(1)
	}
	defer database.DB.Close()

	// verify-stock roda antes de CreateTables: por padrão ele não pode
	// migrar nada, só com --migrate.
	if len(os.Args) > 1 && os.Args[1] == "verify-stock" {
		code := verifyStock(os.Args[2:], os.Stdout)
		database.DB.Close()
		os.Exit(code)
	}

	if err := database.CreateTables(); err != nil {
		fmt.Println("Erro ao preparar as tabelas do banco de dados:", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		runCommand(os.Args[1], os.Args[2:])
		return
	}

	if err := services.SeedDefaultUsers(); err != nil {
		fmt.Println("Erro ao criar usuários padrão:", err)
		os.Exit(1)
	}

	// Um template com erro de sintaxe derruba a inicialização aqui, e não
	// quando alguém abrir aquela tela.
	if err := web.CheckTemplates(); err != nil {
		fmt.Println("Erro nos templates HTML:", err)
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// http.ListenAndServe não tem limite de tempo nenhum: uma conexão que
	// manda o pedido um byte por vez fica aberta para sempre, e muitas
	// delas esgotam o servidor. Os limites abaixo encerram essas conexões.
	// O de escrita tem folga para a exportação de CSV de um histórico grande.
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           web.NewHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	fmt.Println("Servidor web disponível na porta", port)
	if err := server.ListenAndServe(); err != nil {
		fmt.Println("Erro no servidor web:", err)
		os.Exit(1)
	}
}

// runCommand executa um comando administrativo (ver cli.go) e encerra.
func runCommand(name string, args []string) {
	switch name {
	case "reset-password":
		runResetPasswordCommand(args)
	case "create-user":
		runCreateUserCommand(args)
	case "rename-user":
		runRenameUserCommand(args)
	case "change-email":
		runChangeEmailCommand(args)
	default:
		fmt.Println("Comando desconhecido:", name)
		fmt.Println("Comandos: reset-password, create-user, rename-user, change-email, verify-stock")
		os.Exit(1)
	}
}
