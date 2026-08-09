// Binário valheim-webhook — receptor de eventos de um servidor de Valheim.
//
// Ele fica entre três coisas: o container do servidor de jogo (que entrega uma
// linha de log por vez, via hook), o Discord (que anuncia o que aconteceu) e um
// painel web (que mostra quem está no mundo e o que houve). Toda a operação
// passa pela CLI em `cmd/cli`: `serve`, `migrate` e `usuario`.
package main

import "valheim-webhook/cmd/cli"

func main() {
	cli.Execute()
}
