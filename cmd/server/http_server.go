// Package server sobe o servidor HTTP e o encerra com elegância (graceful
// shutdown): o processo para de aceitar conexões novas e ainda devolve os
// requests em voo antes de sair.
//
// O engine (rotas, middlewares, painel) é montado por `cmd/server/routes`; aqui
// fica só o transporte.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"valheim-webhook/internal/pkg/config"
)

// Prazos do servidor HTTP. São tetos de proteção contra cliente lento
// (slowloris) e request travado — não limites de negócio.
const (
	// TempoLeituraCabecalho é o tempo máximo para o cliente enviar os headers.
	TempoLeituraCabecalho = 10 * time.Second
	// TempoLeitura é o tempo máximo para receber o corpo inteiro do request.
	TempoLeitura = 30 * time.Second
	// TempoOcioso é quanto uma conexão keep-alive fica parada antes de cair.
	TempoOcioso = 120 * time.Second
	// TempoEncerramento é quanto o shutdown espera pelos requests em voo.
	TempoEncerramento = 10 * time.Second
	// MaxCabecalho limita o tamanho total dos headers de um request.
	MaxCabecalho = 1 << 20 // 1 MiB
)

// SemPrazoDeEscrita desliga o `WriteTimeout` do servidor.
//
// É a única diferença de transporte em relação ao Atila, e ela é obrigatória
// aqui: o painel mantém uma conexão SSE ABERTA (`/stream`), que por definição
// escreve pouco e durante horas. Com `WriteTimeout` de dois minutos, o servidor
// derrubaria o fluxo ao vivo a cada dois minutos, e o navegador reconectaria
// sem parar.
//
// O que protege contra cliente lento continua de pé: `ReadHeaderTimeout`,
// `ReadTimeout` e `IdleTimeout` — que são justamente os prazos que um SSE não
// atrapalha.
const SemPrazoDeEscrita = 0

// Server é o servidor HTTP da API.
type Server struct {
	http *http.Server
	// endereco só é conhecido depois do Listen (porta 0 nos testes).
	endereco atomic.Pointer[string]
}

// Novo cria o servidor com os prazos padrão, ouvindo na porta configurada.
func Novo(cfg *config.Config, h http.Handler) *Server {
	return &Server{
		http: &http.Server{
			Addr:              cfg.Server.HTTP.Addr(),
			Handler:           h,
			ReadHeaderTimeout: TempoLeituraCabecalho,
			ReadTimeout:       TempoLeitura,
			WriteTimeout:      SemPrazoDeEscrita,
			IdleTimeout:       TempoOcioso,
			MaxHeaderBytes:    MaxCabecalho,
		},
	}
}

// Rodar serve até o contexto ser cancelado (SIGINT/SIGTERM) e então encerra
// com elegância, devolvendo apenas erros reais — encerramento pedido é sucesso.
//
// Bloqueia. O erro de `Listen` (porta ocupada, permissão) volta na hora, antes
// de qualquer request.
func (s *Server) Rodar(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("escutar em %s: %w", s.http.Addr, err)
	}

	endereco := ln.Addr().String()
	s.endereco.Store(&endereco)

	// Buffer de 1: se o servidor morrer depois de o Rodar já ter voltado pelo
	// caminho do shutdown, a goroutine não fica presa no envio.
	erros := make(chan error, 1)
	go func() {
		err := s.http.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil // encerramento pedido, não falha
		}
		erros <- err
	}()

	slog.Info("[HTTP] servidor ouvindo", "endereco", endereco)

	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
	}

	slog.Info("[HTTP] encerrando: aguardando requests em voo", "prazo", TempoEncerramento)

	// Contexto próprio: o do processo já foi cancelado, e é justamente agora
	// que o prazo de drenagem precisa correr.
	//
	// As conexões SSE abertas NÃO drenam sozinhas (elas só terminam quando o
	// cliente desiste), então o prazo vai estourar sempre que houver um painel
	// aberto — e o `Close` a seguir é o que realmente encerra. É esperado.
	prazo, cancelar := context.WithTimeout(context.Background(), TempoEncerramento)
	defer cancelar()

	if err := s.http.Shutdown(prazo); err != nil {
		_ = s.http.Close()
		slog.Warn("[HTTP] prazo de drenagem estourado; conexões fechadas à força", "erro", err)
	}

	// Serve já retornou (ErrServerClosed) — drena o canal para não deixar
	// goroutine viva e propaga qualquer erro inesperado.
	if err := <-erros; err != nil {
		return err
	}

	slog.Info("[HTTP] servidor encerrado")
	return nil
}

// Endereco devolve o endereço efetivo de escuta ("" antes do Listen). Existe
// porque a porta pode ser 0 (o sistema escolhe), o que os testes usam.
func (s *Server) Endereco() string {
	if e := s.endereco.Load(); e != nil {
		return *e
	}
	return ""
}
