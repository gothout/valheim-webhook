package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"valheim-webhook/internal/iam/application/identidade/auth"
	"valheim-webhook/internal/iam/domain/identidade/usuario"
	"valheim-webhook/internal/iam/middleware"
	"valheim-webhook/internal/infra/jwt"
	"valheim-webhook/internal/pkg/papel"
	"valheim-webhook/internal/valheim/application/eventos/ingestao"
	"valheim-webhook/internal/valheim/domain/eventos/evento"
	"valheim-webhook/internal/valheim/domain/mundo/jogador"
)

// Este arquivo é a COSTURA do monólito: cada adaptador liga uma interface
// declarada por um consumidor à implementação de quem sabe fazer o trabalho, e
// traduz o vocabulário de erro de um lado para o do outro.
//
// Ele existe porque nenhum subdomínio importa outro (a regra de camada que
// mantém as peças testáveis em separado). O preço é este arquivo; o ganho é que
// `ingestao` roda em teste sem banco, `auth` sem JWT e o `middleware` sem
// nenhum dos dois.

// ---------- sessão: JWT visto pelo middleware ----------

type sessaoJWT struct{ sessao *jwt.Sessao }

// Validar traduz o vocabulário do emissor para o do middleware.
func (s sessaoJWT) Validar(bruto string) (middleware.Autor, error) {
	claims, err := s.sessao.Validar(bruto)
	switch {
	case errors.Is(err, jwt.ErrTokenAusente):
		return middleware.Autor{}, middleware.ErrSessaoAusente
	case errors.Is(err, jwt.ErrTokenExpirado):
		return middleware.Autor{}, middleware.ErrSessaoExpirada
	case err != nil:
		return middleware.Autor{}, middleware.ErrSessaoInvalida
	}

	id, err := claims.UsuarioUUID()
	if err != nil {
		return middleware.Autor{}, middleware.ErrSessaoInvalida
	}
	// Papel fora do vocabulário (token antigo, papel removido) vira o MENOR
	// acesso, nunca o maior — o middleware confirma o papel real no banco logo
	// em seguida, de qualquer forma.
	p, ok := papel.De(claims.Papel)
	if !ok {
		p = papel.Visualizador
	}

	return middleware.Autor{UUID: id, Nome: claims.Nome, Email: claims.Email, Papel: p}, nil
}

// ---------- conferência da conta: usuário visto pelo middleware ----------

type conferenteDeConta struct{ service usuario.Service }

// Situacao devolve o papel atual e se a conta segue ativa.
func (c conferenteDeConta) Situacao(ctx context.Context, id uuid.UUID) (papel.Papel, bool, error) {
	u, err := c.service.Ler(ctx, id)
	switch {
	case errors.Is(err, usuario.ErrNotFound):
		return "", false, middleware.ErrUsuarioDesconhecido
	case err != nil:
		return "", false, err
	}
	return u.Papel, u.Ativo, nil
}

// ---------- contas: usuário visto pelo caso de uso de sessão ----------

type contasParaAuth struct{ service usuario.Service }

func (a contasParaAuth) Autenticar(ctx context.Context, email, senha string) (auth.Conta, error) {
	u, err := a.service.Autenticar(ctx, email, senha)
	if err != nil {
		return auth.Conta{}, traduzirErroDeConta(err)
	}
	return paraConta(*u), nil
}

func (a contasParaAuth) Ler(ctx context.Context, id uuid.UUID) (auth.Conta, error) {
	u, err := a.service.Ler(ctx, id)
	if err != nil {
		return auth.Conta{}, traduzirErroDeConta(err)
	}
	return paraConta(*u), nil
}

func (a contasParaAuth) RegistrarAcesso(ctx context.Context, id uuid.UUID) error {
	return traduzirErroDeConta(a.service.RegistrarAcesso(ctx, id))
}

func (a contasParaAuth) TrocarSenha(ctx context.Context, id uuid.UUID, senha string) error {
	return traduzirErroDeConta(a.service.TrocarSenha(ctx, id, senha))
}

// paraConta reduz a entidade ao que atravessa a fronteira (sem o hash).
func paraConta(u usuario.Usuario) auth.Conta {
	return auth.Conta{
		UUID:           u.UUID,
		Nome:           u.Nome,
		Email:          u.Email,
		Papel:          u.Papel,
		Ativo:          u.Ativo,
		UltimoAcessoEm: u.UltimoAcessoEm,
	}
}

// traduzirErroDeConta mapeia as sentinelas do subdomínio de usuários para as do
// caso de uso de sessão.
func traduzirErroDeConta(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, usuario.ErrCredenciais):
		return auth.ErrCredenciais
	case errors.Is(err, usuario.ErrContaInativa):
		return auth.ErrContaInativa
	case errors.Is(err, usuario.ErrNotFound):
		return auth.ErrContaDesconhecida
	case errors.Is(err, usuario.ErrSenhaFraca):
		return auth.ErrSenhaFraca
	case errors.Is(err, usuario.ErrPersistencia):
		return fmt.Errorf("%w: %v", auth.ErrIndisponivel, err)
	default:
		return err
	}
}

// ---------- eventos: diário visto pela ingestão ----------

type eventosParaIngestao struct{ service evento.Service }

func (e eventosParaIngestao) Registrar(ctx context.Context, servidor, linha string) (ingestao.Registro, error) {
	registrado, err := e.service.Registrar(ctx, servidor, linha)
	switch {
	case errors.Is(err, evento.ErrLinhaVazia):
		return ingestao.Registro{}, ingestao.ErrCorpoVazio
	case errors.Is(err, evento.ErrLinhaLonga):
		return ingestao.Registro{}, ingestao.ErrCorpoGrande
	case err != nil:
		return ingestao.Registro{}, fmt.Errorf("%w: %v", ingestao.ErrRegistro, err)
	}

	return ingestao.Registro{
		UUID:       registrado.UUID,
		Servidor:   registrado.Servidor,
		Tipo:       string(registrado.Tipo),
		Rotulo:     registrado.Tipo.Rotulo(),
		Jogador:    registrado.Jogador,
		SteamID:    registrado.SteamID,
		ZDOID:      registrado.ZDOID,
		Texto:      registrado.Texto,
		Linha:      registrado.Linha,
		OcorridoEm: registrado.OcorridoEm,
	}, nil
}

// ---------- personagens: saldo visto pela ingestão ----------

type jogadoresParaIngestao struct{ service jogador.Service }

func (j jogadoresParaIngestao) Entrou(ctx context.Context, servidor, nome, zdoid string, quando time.Time) (ingestao.Presenca, error) {
	p, transicao, err := j.service.RegistrarEntrada(ctx, servidor, nome, zdoid, quando)
	return paraPresenca(p, transicao, nome), traduzirErroDePresenca(err)
}

func (j jogadoresParaIngestao) SaiuPorZDOID(ctx context.Context, servidor, zdoid string, quando time.Time) (ingestao.Presenca, error) {
	p, transicao, err := j.service.RegistrarSaidaPorZDOID(ctx, servidor, zdoid, quando)
	return paraPresenca(p, transicao, ""), traduzirErroDePresenca(err)
}

func (j jogadoresParaIngestao) SaiuPorNome(ctx context.Context, servidor, nome string, quando time.Time) (ingestao.Presenca, error) {
	p, transicao, err := j.service.RegistrarSaida(ctx, servidor, nome, quando)
	return paraPresenca(p, transicao, nome), traduzirErroDePresenca(err)
}

func (j jogadoresParaIngestao) Morreu(ctx context.Context, servidor, nome string, quando time.Time) error {
	_, err := j.service.RegistrarMorte(ctx, servidor, nome, quando)
	return traduzirErroDePresenca(err)
}

func (j jogadoresParaIngestao) Atividade(ctx context.Context, servidor, nome string, quando time.Time) error {
	_, err := j.service.RegistrarAtividade(ctx, servidor, nome, quando)
	return traduzirErroDePresenca(err)
}

func (j jogadoresParaIngestao) ServidorReiniciou(ctx context.Context, servidor string, quando time.Time) (int64, error) {
	derrubados, err := j.service.ReiniciarServidor(ctx, servidor, quando)
	return derrubados, traduzirErroDePresenca(err)
}

// paraPresenca reduz a entidade ao que a ingestão precisa saber.
func paraPresenca(j *jogador.Jogador, transicao jogador.Transicao, nomeReserva string) ingestao.Presenca {
	if j == nil {
		return ingestao.Presenca{Nome: nomeReserva}
	}
	return ingestao.Presenca{Nome: j.Nome, Online: j.Online, Mudou: bool(transicao)}
}

// traduzirErroDePresenca separa a AUSÊNCIA (esperada) da falha (não esperada).
func traduzirErroDePresenca(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, jogador.ErrNotFound), errors.Is(err, jogador.ErrNomeVazio):
		return ingestao.ErrPresencaDesconhecida
	default:
		return err
	}
}
