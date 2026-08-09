// Package jwt emite e valida o token da sessão do painel.
//
// O token é um JWT HS256 assinado com `security.jwt_secret`. Ele viaja num
// cookie `HttpOnly` (o painel nunca o lê em JavaScript) e também é aceito no
// header `Authorization: Bearer` — a segunda forma existe para script e `curl`,
// não para o navegador.
//
// Não há refresh token, e é uma decisão: refresh serve para manter sessão longa
// com token curto, ao custo de uma denylist e de um segundo segredo. Aqui a
// sessão dura uma semana e o custo de logar de novo é abrir o painel e digitar
// a senha — o preço do refresh é maior que o problema que ele resolveria.
//
// Esta dependência é FATAL: sem emissor de token não há login, e sem login o
// painel inteiro é inacessível.
package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"valheim-webhook/internal/pkg/config"
)

// Emissor é o nome que assina os tokens (claim `iss`).
const Emissor = "valheim-webhook"

// TamanhoDoSegredoSorteado é o tamanho, em bytes, do segredo gerado quando a
// configuração não traz um.
const TamanhoDoSegredoSorteado = 32

// Claims é o conteúdo do token.
//
// Papel e nome viajam DENTRO do token de propósito: sem eles, toda requisição
// autenticada precisaria de uma ida ao banco só para descobrir quem é o dono da
// sessão. O preço é conhecido e aceito — trocar o papel de um usuário só passa
// a valer no próximo login dele (ver o comentário de Sessao).
type Claims struct {
	Nome  string `json:"nome"`
	Email string `json:"email"`
	Papel string `json:"papel"`
	jwt.RegisteredClaims
}

// Sessao é o emissor/validador do processo.
type Sessao struct {
	segredo   []byte
	validade  time.Duration
	sorteado  bool
	analisado *jwt.Parser
}

// Novo monta o emissor.
//
// Segredo vazio é aceito e SORTEADO — com aviso. É o que permite subir o
// serviço em casa sem escolher um segredo antes de ver a tela, mantendo a
// propriedade que importa: o token continua sendo impossível de forjar. O que
// se perde é a continuidade da sessão entre reinícios.
func Novo(cfg config.Security) (*Sessao, error) {
	segredo := strings.TrimSpace(cfg.JWTSecret)
	sorteado := false

	if segredo == "" {
		gerado, err := SegredoAleatorio()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSegredo, err)
		}
		segredo, sorteado = gerado, true
	}

	validade := cfg.SessaoExpiracao()
	if validade <= 0 {
		validade = 168 * time.Hour
	}

	return &Sessao{
		segredo:  []byte(segredo),
		validade: validade,
		sorteado: sorteado,
		// O algoritmo é fixado na validação: sem isto, um token forjado com
		// `alg: none` (ou com HMAC sobre uma chave pública) seria aceito. É a
		// falha clássica de JWT e não se resolve depois.
		analisado: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(Emissor),
			jwt.WithExpirationRequired(),
		),
	}, nil
}

// Sorteado diz se o segredo foi gerado no boot (o boot loga um aviso).
func (s *Sessao) Sorteado() bool { return s != nil && s.sorteado }

// Validade é quanto tempo um token novo dura.
func (s *Sessao) Validade() time.Duration {
	if s == nil {
		return 0
	}
	return s.validade
}

// Emitir cria o token da sessão de um usuário.
func (s *Sessao) Emitir(usuarioUUID uuid.UUID, nome, email, papel string) (string, time.Time, error) {
	if s == nil {
		return "", time.Time{}, ErrNotInitialized
	}

	agora := time.Now().UTC()
	expira := agora.Add(s.validade)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Nome:  nome,
		Email: email,
		Papel: papel,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Emissor,
			Subject:   usuarioUUID.String(),
			IssuedAt:  jwt.NewNumericDate(agora),
			NotBefore: jwt.NewNumericDate(agora),
			ExpiresAt: jwt.NewNumericDate(expira),
			ID:        uuid.NewString(),
		},
	})

	assinado, err := token.SignedString(s.segredo)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %v", ErrAssinatura, err)
	}
	return assinado, expira, nil
}

// Validar confere o token e devolve o conteúdo.
//
// Toda recusa vira ErrTokenInvalido ou ErrTokenExpirado — nunca o erro cru da
// biblioteca, que descreveria ao cliente exatamente o que faltou para o token
// passar.
func (s *Sessao) Validar(bruto string) (*Claims, error) {
	if s == nil {
		return nil, ErrNotInitialized
	}
	limpo := strings.TrimSpace(bruto)
	if limpo == "" {
		return nil, ErrTokenAusente
	}

	claims := &Claims{}
	_, err := s.analisado.ParseWithClaims(limpo, claims, func(*jwt.Token) (any, error) {
		return s.segredo, nil
	})
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return nil, ErrTokenExpirado
	case err != nil:
		return nil, ErrTokenInvalido
	}

	if _, err := uuid.Parse(claims.Subject); err != nil {
		return nil, ErrTokenInvalido
	}
	return claims, nil
}

// UsuarioUUID extrai o identificador do usuário do token já validado.
func (c *Claims) UsuarioUUID() (uuid.UUID, error) {
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, ErrTokenInvalido
	}
	return id, nil
}

// SegredoAleatorio gera um segredo de assinatura.
func SegredoAleatorio() (string, error) {
	bytes := make([]byte, TamanhoDoSegredoSorteado)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
