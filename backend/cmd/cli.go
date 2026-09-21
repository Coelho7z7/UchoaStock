package main

import (
	"fmt"
	"os"

	"uchoastock/backend/services"
)

// Comandos administrativos de linha de comando. Cada um executa e encerra
// o processo, sem subir o servidor web (ver runCommand, em main.go).

// runResetPasswordCommand troca a senha de uma conta já existente via
// linha de comando, ex.:
//
//	go run ./backend/cmd reset-password superadmin@gmail.com NovaSenha!123
//
// Encerra o processo sem subir o servidor web. Existe porque ainda não há
// uma tela no painel para trocar a senha de um usuário já criado (só na
// criação e no seed inicial).
func runResetPasswordCommand(args []string) {
	if len(args) != 2 {
		fmt.Println("Uso: reset-password <email> <nova-senha>")
		os.Exit(1)
	}

	if err := services.ResetPassword(args[0], args[1]); err != nil {
		fmt.Println("Erro ao trocar a senha:", err)
		os.Exit(1)
	}

	fmt.Println("Senha atualizada com sucesso para", args[0])
}

// runCreateUserCommand cadastra um usuário via linha de comando, ex.:
//
//	go run ./backend/cmd create-user "Nome" email@gmail.com "Senha!123" admin
//
// Cargos aceitos: admin, gestor, almoxarife, solicitante, auditor (nunca "superadmin" — reservado a
// superadmin@gmail.com e criado apenas pelo seed). Encerra o processo sem subir
// o servidor web.
func runCreateUserCommand(args []string) {
	if len(args) != 4 {
		fmt.Println("Uso: create-user <nome> <email> <senha> <admin|gestor|almoxarife|solicitante|auditor>")
		os.Exit(1)
	}

	if err := services.CreateUserWeb(args[0], args[1], args[2], args[3], 0); err != nil {
		fmt.Println("Erro ao criar usuário:", err)
		os.Exit(1)
	}

	fmt.Println("Usuário criado com sucesso:", args[1])
}

// runRenameUserCommand troca o nome de exibição de uma conta já existente
// via linha de comando, ex.:
//
//	go run ./backend/cmd rename-user matheus@gmail.com "Novo Nome"
//
// Encerra o processo sem subir o servidor web.
func runRenameUserCommand(args []string) {
	if len(args) != 2 {
		fmt.Println("Uso: rename-user <email> <novo-nome>")
		os.Exit(1)
	}

	if err := services.RenameUser(args[0], args[1]); err != nil {
		fmt.Println("Erro ao renomear usuário:", err)
		os.Exit(1)
	}

	fmt.Println("Nome atualizado com sucesso para", args[0])
}

// runChangeEmailCommand troca o email de uma conta já existente via linha
// de comando, ex.:
//
//	go run ./backend/cmd change-email matheus@gmail.com gerente@gmail.com
//
// Encerra o processo sem subir o servidor web.
func runChangeEmailCommand(args []string) {
	if len(args) != 2 {
		fmt.Println("Uso: change-email <email-atual> <novo-email>")
		os.Exit(1)
	}

	if err := services.ChangeUserEmail(args[0], args[1]); err != nil {
		fmt.Println("Erro ao trocar email:", err)
		os.Exit(1)
	}

	fmt.Println("Email atualizado com sucesso:", args[0], "->", args[1])
}
