package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

	// Desligamento gracioso. Num redeploy o Railway manda o sinal SIGTERM
	// antes de matar o processo; o Ctrl+C no terminal manda o SIGINT. Sem
	// tratar o sinal, o processo morria na hora: uma movimentação no meio
	// do caminho perdia a resposta (a pessoa não sabia se tinha gravado) e
	// o banco nunca era fechado.
	//
	// signal.NotifyContext devolve um contexto que é "cancelado" quando um
	// desses sinais chega. O servidor roda numa goroutine (uma função que
	// executa em paralelo) para o main ficar livre esperando o sinal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		fmt.Println("Servidor web disponível na porta", port)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		// Nem chegou a subir (porta ocupada, por exemplo).
		fmt.Println("Erro no servidor web:", err)
		database.DB.Close()
		os.Exit(1)
	case <-ctx.Done():
	}

	// Shutdown para de aceitar conexões novas e espera as requisições em
	// andamento terminarem, até o limite: depois dele, encerra assim mesmo.
	// O limite fica abaixo dos 30 segundos que o Railway espera antes de
	// forçar o fim do processo.
	fmt.Println("Sinal de encerramento recebido: terminando as requisições em andamento...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Println("Erro ao encerrar o servidor web:", err)
	}
	fmt.Println("Servidor encerrado.")
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
