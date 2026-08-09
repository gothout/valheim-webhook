// admin.js — a tela de administração: usuários, integração com o Discord e o
// log do processo.
//
// Toda a tela é restrita a administradores. O servidor já barra (403 nas
// rotas); aqui a verificação é para a pessoa não ficar olhando uma tela vazia
// sem entender por quê.

let logUltimaSeq = 0;
let logTimer = null;

document.addEventListener("DOMContentLoaded", async () => {
  marcarAbaAtiva();
  ligarSair();

  try {
    await exigirSessao({ exigeAdmin: true });
  } catch {
    return;
  }

  ligarFormularios();
  await Promise.all([carregarUsuarios(), carregarDiscord(), carregarLogs()]);
  logTimer = setInterval(carregarLogs, 5000);
});

window.addEventListener("beforeunload", () => clearInterval(logTimer));

// ---------- usuários ----------

async function carregarUsuarios() {
  try {
    const usuarios = await pedir(API.usuarios);
    const corpo = document.querySelector("#tabela-usuarios tbody");
    corpo.innerHTML = "";

    usuarios.forEach((u) => {
      const linha = document.createElement("tr");
      linha.innerHTML = `
        <td>${escapar(u.nome)}</td>
        <td>${escapar(u.email)}</td>
        <td>${escapar(u.papel_rotulo)}</td>
        <td>${u.ativo ? '<span class="selo ligado">ativo</span>' : '<span class="selo desligado">inativo</span>'}</td>
        <td>${escapar(u.ultimo_acesso_em ? formatarHora(u.ultimo_acesso_em) : "nunca")}</td>
        <td></td>`;

      linha.querySelector("td:last-child").append(
        botao("Senha", () => trocarSenhaDe(u)),
        botao(u.ativo ? "Desativar" : "Ativar", () => alternarSituacao(u)),
        botao("Remover", () => removerUsuario(u), "perigo"),
      );
      corpo.appendChild(linha);
    });
  } catch (erro) {
    avisar("#aviso-usuarios", `Não foi possível carregar os usuários: ${erro.message}`);
  }
}

function botao(texto, aoClicar, classe = "") {
  const b = document.createElement("button");
  b.textContent = texto;
  b.className = classe;
  b.style.marginRight = "6px";
  b.addEventListener("click", aoClicar);
  return b;
}

async function criarUsuario(evento) {
  evento.preventDefault();
  const form = evento.target;
  const corpo = {
    nome: form.nome.value,
    email: form.email.value,
    senha: form.senha.value,
    papel: form.papel.value,
  };

  try {
    await pedir(API.usuarios, { method: "POST", body: JSON.stringify(corpo) });
    form.reset();
    avisar("#aviso-usuarios", "Usuário criado.", "ok");
    await carregarUsuarios();
  } catch (erro) {
    avisar("#aviso-usuarios", erro.message);
  }
}

async function alternarSituacao(u) {
  try {
    await pedir(`${API.usuarios}/${u.uuid}`, {
      method: "PATCH",
      body: JSON.stringify({ nome: u.nome, papel: u.papel, ativo: !u.ativo }),
    });
    await carregarUsuarios();
  } catch (erro) {
    avisar("#aviso-usuarios", erro.message);
  }
}

async function removerUsuario(u) {
  if (!confirm(`Remover ${u.nome} (${u.email})? Isto não tem volta.`)) return;
  try {
    await pedir(`${API.usuarios}/${u.uuid}`, { method: "DELETE" });
    avisar("#aviso-usuarios", "Usuário removido.", "ok");
    await carregarUsuarios();
  } catch (erro) {
    avisar("#aviso-usuarios", erro.message);
  }
}

async function trocarSenhaDe(u) {
  const senha = prompt(`Nova senha para ${u.nome} (mínimo 8 caracteres):`);
  if (!senha) return;
  try {
    await pedir(`${API.usuarios}/${u.uuid}/senha`, {
      method: "PATCH",
      body: JSON.stringify({ senha }),
    });
    avisar("#aviso-usuarios", "Senha trocada.", "ok");
  } catch (erro) {
    avisar("#aviso-usuarios", erro.message);
  }
}

// ---------- integração com o Discord ----------

async function carregarDiscord() {
  try {
    const cfg = await pedir(API.discord);
    const form = document.querySelector("#form-discord");

    form.habilitado.checked = cfg.habilitado;
    form.modo.value = cfg.modo;
    form.username.value = cfg.username || "";
    form.canal_id.value = cfg.canal_id || "";

    // Os segredos NUNCA voltam do servidor. O campo fica vazio, com a máscara
    // no placeholder: deixar em branco mantém o que está gravado.
    form.webhook_url.value = "";
    form.webhook_url.placeholder = cfg.webhook_configurado
      ? `configurado (${cfg.webhook_mascara}) — deixe em branco para manter`
      : "https://discord.com/api/webhooks/...";
    form.bot_token.value = "";
    form.bot_token.placeholder = cfg.token_configurado
      ? `configurado (${cfg.token_mascara}) — deixe em branco para manter`
      : "token do bot";

    document.querySelectorAll("[name='eventos']").forEach((caixa) => {
      caixa.checked = (cfg.eventos || []).includes(caixa.value);
    });

    const selo = document.querySelector("#selo-discord");
    selo.textContent = cfg.ativa ? "ativa" : "desligada";
    selo.className = `selo ${cfg.ativa ? "ligado" : "desligado"}`;

    alternarCamposDoModo();
  } catch (erro) {
    avisar("#aviso-discord", `Não foi possível carregar a integração: ${erro.message}`);
  }
}

async function salvarDiscord(evento) {
  evento.preventDefault();
  const form = evento.target;

  const eventos = [...document.querySelectorAll("[name='eventos']:checked")].map((c) => c.value);
  const corpo = {
    habilitado: form.habilitado.checked,
    modo: form.modo.value,
    username: form.username.value,
    eventos,
  };

  // Campo em branco = "mantenha o que está gravado", e por isso ele nem entra
  // no corpo: o servidor distingue ausente (manter) de vazio (apagar).
  if (form.webhook_url.value.trim() !== "") corpo.webhook_url = form.webhook_url.value.trim();
  if (form.bot_token.value.trim() !== "") corpo.bot_token = form.bot_token.value.trim();
  corpo.canal_id = form.canal_id.value.trim();

  try {
    await pedir(API.discord, { method: "PUT", body: JSON.stringify(corpo) });
    avisar("#aviso-discord", "Integração salva e aplicada.", "ok");
    await carregarDiscord();
  } catch (erro) {
    avisar("#aviso-discord", erro.message);
  }
}

async function testarDiscord() {
  try {
    const resposta = await pedir(`${API.discord}/teste`, { method: "POST" });
    avisar("#aviso-discord", resposta.mensagem, "ok");
  } catch (erro) {
    avisar("#aviso-discord", erro.message);
  }
}

function alternarCamposDoModo() {
  const modo = document.querySelector("#form-discord").modo.value;
  document.querySelector("#campos-webhook").hidden = modo !== "webhook";
  document.querySelector("#campos-bot").hidden = modo !== "bot";
}

// ---------- logs ----------

async function carregarLogs() {
  const nivel = document.querySelector("#log-nivel").value;
  const busca = document.querySelector("#log-busca").value.trim();

  // Filtro mudou = recomeça do zero; senão, pede só o que apareceu depois da
  // última linha já exibida.
  const chave = `${nivel}|${busca}`;
  if (chave !== carregarLogs.ultimaChave) {
    carregarLogs.ultimaChave = chave;
    logUltimaSeq = 0;
    document.querySelector("#logs").innerHTML = "";
  }

  const parametros = new URLSearchParams({ depois_de: String(logUltimaSeq) });
  if (nivel) parametros.set("nivel", nivel);
  if (busca) parametros.set("busca", busca);

  try {
    const trilha = await pedir(`${API.logs}?${parametros}`);
    const caixa = document.querySelector("#logs");
    const coladoNoFim = caixa.scrollTop + caixa.clientHeight >= caixa.scrollHeight - 30;

    trilha.linhas.forEach((linha) => caixa.appendChild(linhaDeLog(linha)));
    logUltimaSeq = trilha.ultima_seq;

    // Só rola sozinho se a pessoa já estava no fim: rolar para baixo enquanto
    // alguém lê uma linha antiga é a forma mais rápida de tornar o painel
    // inútil.
    if (coladoNoFim) caixa.scrollTop = caixa.scrollHeight;
  } catch (erro) {
    avisar("#aviso-logs", `Não foi possível ler o log: ${erro.message}`);
  }
}

function linhaDeLog(linha) {
  const div = document.createElement("div");
  div.className = "linha-log";
  const atributos = Object.entries(linha.atributos || {})
    .map(([chave, valor]) => `${chave}=${valor}`)
    .join(" ");
  div.innerHTML = `
    <span class="hora">${escapar(formatarHora(linha.ts))}</span>
    <span class="nivel ${escapar(linha.nivel)}">${escapar(linha.nivel)}</span>
    <span class="msg">${escapar(linha.mensagem)} <span class="attrs">${escapar(atributos)}</span></span>`;
  return div;
}

// ---------- ligações ----------

function ligarFormularios() {
  document.querySelector("#form-usuario").addEventListener("submit", criarUsuario);
  document.querySelector("#form-discord").addEventListener("submit", salvarDiscord);
  document.querySelector("#form-discord").modo.addEventListener("change", alternarCamposDoModo);
  document.querySelector("#botao-testar").addEventListener("click", testarDiscord);
  document.querySelector("#log-nivel").addEventListener("change", carregarLogs);
  document.querySelector("#log-busca").addEventListener("change", carregarLogs);
  document.querySelector("#form-minha-senha").addEventListener("submit", trocarMinhaSenha);
}

async function trocarMinhaSenha(evento) {
  evento.preventDefault();
  const form = evento.target;
  try {
    await pedir(API.minhaSenha, {
      method: "PATCH",
      body: JSON.stringify({
        senha_atual: form.senha_atual.value,
        senha_nova: form.senha_nova.value,
      }),
    });
    form.reset();
    avisar("#aviso-minha-senha", "Senha trocada.", "ok");
  } catch (erro) {
    avisar("#aviso-minha-senha", erro.message);
  }
}
