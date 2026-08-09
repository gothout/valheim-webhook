// painel.js — a tela principal: quem está no mundo, os números do dia e o feed
// ao vivo.
//
// O feed chega por SSE. A lista de jogadores e os números são recarregados
// quando um evento MUDA presença — e não a cada evento: uma conversa animada no
// chat não precisa recarregar a lista de quem está online.

const FEED_MAXIMO = 200;

let jogadoresCarregando = false;

document.addEventListener("DOMContentLoaded", async () => {
  marcarAbaAtiva();
  ligarSair();

  try {
    await exigirSessao();
  } catch {
    return; // exigirSessao já redirecionou
  }

  await Promise.all([carregarResumo(), carregarJogadores(), carregarFeed()]);
  abrirFluxo();
});

// ---------- carga inicial ----------

async function carregarResumo() {
  try {
    const resumo = await pedir(`${API.resumo}?horas=24`);
    const porTipo = resumo.por_tipo || {};
    definir("total", resumo.total);
    definir("entradas", porTipo.entrou || 0);
    definir("mortes", porTipo.morreu || 0);
    definir("falas", porTipo.mensagem || 0);
  } catch (erro) {
    avisar("#aviso-painel", `Não foi possível carregar o resumo: ${erro.message}`);
  }
}

async function carregarJogadores() {
  if (jogadoresCarregando) return;
  jogadoresCarregando = true;
  try {
    const jogadores = await pedir(API.jogadores);
    desenharJogadores(jogadores);
  } catch (erro) {
    avisar("#aviso-painel", `Não foi possível carregar os personagens: ${erro.message}`);
  } finally {
    jogadoresCarregando = false;
  }
}

async function carregarFeed() {
  try {
    const pagina = await pedir(`${API.eventos}?pageSize=100`);
    const lista = document.querySelector("#feed");
    lista.innerHTML = "";
    // A API devolve do mais recente para o mais antigo; o feed mostra na mesma
    // ordem, então basta anexar.
    pagina.items.forEach((evento) => lista.appendChild(linhaDoFeed(evento)));
    if (pagina.items.length === 0) mostrarVazio(lista);
  } catch (erro) {
    avisar("#aviso-painel", `Não foi possível carregar os eventos: ${erro.message}`);
  }
}

// ---------- tempo real ----------

function abrirFluxo() {
  const fluxo = new EventSource(API.stream);
  const sinal = document.querySelector("#sinal-fluxo");

  fluxo.addEventListener("conectado", () => {
    sinal.textContent = "ao vivo";
    sinal.className = "selo ligado";
  });

  fluxo.addEventListener("evento", (mensagem) => {
    const evento = JSON.parse(mensagem.data);
    anexarAoFeed(evento);
    if (evento.mudou_presenca) {
      carregarJogadores();
      carregarResumo();
    }
  });

  // O navegador reconecta sozinho (o servidor manda `retry: 3000`); o que
  // fazemos aqui é só contar ao usuário que a tela está desatualizada.
  fluxo.onerror = () => {
    sinal.textContent = "reconectando…";
    sinal.className = "selo desligado";
  };
}

function anexarAoFeed(evento) {
  const lista = document.querySelector("#feed");
  const vazio = lista.querySelector(".vazio");
  if (vazio) vazio.remove();

  const linha = linhaDoFeed(evento);
  linha.classList.add("novo");
  lista.prepend(linha);

  while (lista.children.length > FEED_MAXIMO) {
    lista.removeChild(lista.lastChild);
  }
}

// ---------- desenho ----------

function linhaDoFeed(evento) {
  const li = document.createElement("li");
  li.innerHTML = `
    <span class="hora">${escapar(formatarHora(evento.ocorrido_em))}</span>
    <span class="etiqueta ${escapar(evento.tipo)}">${escapar(evento.rotulo || evento.tipo)}</span>
    <span class="descricao">${descrever(evento)}</span>`;
  return li;
}

// descrever monta a frase do evento. Tudo o que vem do jogo passa por escapar.
function descrever(evento) {
  const quem = evento.jogador ? `<span class="quem">${escapar(evento.jogador)}</span>` : "";
  switch (evento.tipo) {
    case "entrou":
      return `${quem} entrou no mundo`;
    case "saiu":
      return quem ? `${quem} saiu` : "um personagem saiu";
    case "morreu":
      return `${quem} morreu`;
    case "mensagem":
      return `${quem} <span class="fala">${escapar(evento.texto || "")}</span>`;
    case "servidor_pronto":
      return "servidor no ar";
    case "conexao":
      return "nova conexão com o servidor";
    default:
      return `<span class="fala">${escapar(evento.texto || evento.linha || "")}</span>`;
  }
}

function desenharJogadores(jogadores) {
  const caixa = document.querySelector("#jogadores");
  caixa.innerHTML = "";

  if (!jogadores || jogadores.length === 0) {
    mostrarVazio(caixa, "Nenhum personagem visto ainda.");
    definir("online", 0);
    return;
  }

  let online = 0;
  jogadores.forEach((j) => {
    if (j.online) online += 1;
    const item = document.createElement("div");
    item.className = `jogador${j.online ? " online" : ""}`;
    item.innerHTML = `
      <span class="ponto"></span>
      <span>
        <div class="nome">${escapar(j.nome)}</div>
        <div class="detalhe">${detalheDoJogador(j)}</div>
      </span>`;
    caixa.appendChild(item);
  });
  definir("online", online);
}

function detalheDoJogador(j) {
  const tempo = formatarDuracao(j.tempo_total_seg);
  if (j.online) {
    return `desde ${escapar(formatarHora(j.entrou_em))} · ${escapar(tempo)} no total · ${j.mortes} morte(s)`;
  }
  return `visto em ${escapar(formatarHora(j.ultimo_em))} · ${escapar(tempo)} no total · ${j.mortes} morte(s)`;
}

function mostrarVazio(alvo, texto = "Nada por aqui ainda.") {
  const vazio = document.createElement("div");
  vazio.className = "vazio";
  vazio.textContent = texto;
  alvo.appendChild(vazio);
}

function definir(nome, valor) {
  const alvo = document.querySelector(`[data-indicador="${nome}"]`);
  if (alvo) alvo.textContent = valor;
}
