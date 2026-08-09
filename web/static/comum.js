// comum.js — o que as três telas compartilham: falar com a API, tratar sessão
// e formatar as coisas que aparecem em todas elas.
//
// Sem framework e sem bundler. O painel tem três telas e uma dúzia de
// chamadas; a dependência que não existe é a que nunca vai quebrar num
// `npm audit` daqui a dois anos.

const API = {
  eventos: "/api/domain/eventos/eventos",
  resumo: "/api/domain/eventos/resumo",
  jogadores: "/api/domain/mundo/jogadores",
  usuarios: "/api/domain/identidade/usuarios",
  discord: "/api/domain/configuracao/discord",
  logs: "/api/domain/observabilidade/logs",
  eu: "/api/application/identidade/auth/eu",
  login: "/api/application/identidade/auth/login",
  logout: "/api/application/identidade/auth/logout",
  minhaSenha: "/api/application/identidade/auth/eu/senha",
  stream: "/api/application/eventos/ingestao/stream",
  status: "/api/status",
};

// pedir é o único ponto de contato com a API.
//
// Ele centraliza três coisas que, espalhadas, viram bug: o cookie de sessão
// (`same-origin`), a resposta 401 (que manda para o login em vez de deixar a
// tela quebrada) e o corpo de erro padrão da API, que sempre tem `message`.
async function pedir(url, opcoes = {}) {
  const resposta = await fetch(url, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(opcoes.headers || {}) },
    ...opcoes,
  });

  if (resposta.status === 401 && !url.endsWith("/login")) {
    window.location.href = "/login";
    throw new Error("sessão expirada");
  }

  if (resposta.status === 204) return null;

  const texto = await resposta.text();
  const corpo = texto ? JSON.parse(texto) : null;

  if (!resposta.ok) {
    const erro = new Error((corpo && corpo.message) || `erro ${resposta.status}`);
    erro.status = resposta.status;
    erro.rayTrace = corpo && corpo.ray_trace;
    throw erro;
  }
  return corpo;
}

// exigirSessao carrega o usuário logado e desenha o cabeçalho.
// Redireciona para o login quando não há sessão.
async function exigirSessao({ exigeAdmin = false } = {}) {
  const eu = await pedir(API.eu);

  const alvo = document.querySelector("[data-usuario]");
  if (alvo) alvo.textContent = eu.nome;

  document.querySelectorAll("[data-so-admin]").forEach((el) => {
    el.hidden = !eu.admin;
  });

  if (exigeAdmin && !eu.admin) {
    window.location.href = "/";
    throw new Error("acesso restrito a administradores");
  }
  return eu;
}

// ligarSair pendura o logout no botão do cabeçalho.
function ligarSair() {
  const botao = document.querySelector("[data-sair]");
  if (!botao) return;
  botao.addEventListener("click", async () => {
    try {
      await pedir(API.logout, { method: "POST" });
    } finally {
      window.location.href = "/login";
    }
  });
}

// marcarAbaAtiva destaca a aba correspondente à página aberta.
function marcarAbaAtiva() {
  const atual = window.location.pathname;
  document.querySelectorAll("nav.abas a").forEach((a) => {
    if (a.getAttribute("href") === atual) a.classList.add("ativa");
  });
}

// ---------- formatação ----------

const HORA = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit",
});

function formatarHora(iso) {
  if (!iso) return "—";
  return HORA.format(new Date(iso));
}

// duracao transforma segundos em "3h 12min" — a forma como alguém fala do
// tempo que passou jogando.
function formatarDuracao(segundos) {
  if (!segundos || segundos < 60) return `${Math.max(0, Math.round(segundos || 0))}s`;
  const horas = Math.floor(segundos / 3600);
  const minutos = Math.floor((segundos % 3600) / 60);
  if (horas === 0) return `${minutos}min`;
  return `${horas}h ${minutos}min`;
}

// escapar é obrigatório em tudo o que vem do jogo: nome de personagem e texto
// de chat são escolhidos por quem joga, e vão parar dentro do HTML da página.
function escapar(texto) {
  const div = document.createElement("div");
  div.textContent = texto == null ? "" : String(texto);
  return div.innerHTML;
}

// avisar mostra uma mensagem no topo do bloco (erro ou sucesso).
function avisar(seletor, mensagem, tipo = "erro") {
  const caixa = document.querySelector(seletor);
  if (!caixa) return;
  if (!mensagem) {
    caixa.hidden = true;
    return;
  }
  caixa.hidden = false;
  caixa.className = `aviso ${tipo}`;
  caixa.textContent = mensagem;
}
