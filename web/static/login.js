// login.js — a única tela que existe sem sessão.

document.addEventListener("DOMContentLoaded", () => {
  const form = document.querySelector("#form-login");
  const botao = form.querySelector("button[type='submit']");

  form.addEventListener("submit", async (evento) => {
    evento.preventDefault();
    avisar("#aviso-login", "");
    botao.disabled = true;

    try {
      await pedir(API.login, {
        method: "POST",
        body: JSON.stringify({ email: form.email.value, senha: form.senha.value }),
      });
      window.location.href = "/";
    } catch (erro) {
      // A API não distingue "e-mail não existe" de "senha errada", e a tela
      // repete a mensagem dela tal como veio — inventar um texto mais
      // específico aqui desfaria a proteção do outro lado.
      avisar("#aviso-login", erro.message);
      form.senha.value = "";
      form.senha.focus();
    } finally {
      botao.disabled = false;
    }
  });
});
