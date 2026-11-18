import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Política de privacidade · M80",
  description: "Como o M80 e a extensão M80 Extrato tratam os dados das contas Azul e Smiles.",
}

// Public on purpose (proxy.ts only guards /dashboard and /portal): the Chrome Web Store links to
// it from the M80 Extrato listing, and a reviewer has no M80 login. Keep it in step with what the
// extension actually does - the store rejects a policy that does not match the code.
export default function PrivacyPolicyPage() {
  return (
    <div className="min-h-screen bg-white">
      <article className="mx-auto flex w-full max-w-2xl flex-col gap-5 px-4 py-10 text-sm leading-relaxed text-slate-700">
        <header className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold text-slate-900">Política de privacidade</h1>
          <p className="text-slate-500">M80 e extensão M80 Extrato · atualizada em 17/11/2026</p>
        </header>

        <section className="flex flex-col gap-2">
          <h2 className="text-base font-semibold text-slate-900">O que é</h2>
          <p>
            O M80 é um sistema de gestão de milhas. O portal do M80 mostra a clientes convidados
            o extrato das contas Azul Fidelidade e Smiles que foram liberadas para eles. A
            extensão de navegador <strong>M80 Extrato</strong> existe só para conectar essas
            contas ao portal: ela abre a página da companhia no navegador do próprio usuário
            para ele receber o código de verificação da Smiles ou fazer o login na Azul, e
            devolve a conexão ao M80.
          </p>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="text-base font-semibold text-slate-900">Quais dados a extensão trata</h2>
          <ul className="list-disc space-y-1 pl-5">
            <li>
              <strong>Smiles:</strong> o código temporário que a Smiles devolve ao pedir o envio
              do código de verificação. A senha da Smiles não passa pela extensão.
            </li>
            <li>
              <strong>Azul:</strong> o CPF usado no login e a chave de sessão que o site da Azul
              cria depois que o próprio usuário faz o login na aba aberta pela extensão. A
              extensão não lê nem guarda a senha digitada.
            </li>
          </ul>
          <p>
            Esses dados são enviados apenas ao portal M80 em que o usuário está logado, e só
            quando ele clica em Conectar ou Pelo navegador. A extensão age apenas em abas que
            ela mesma abriu para essa conexão. Ela não guarda nada no navegador, não lê outros
            sites e não coleta histórico de navegação.
          </p>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="text-base font-semibold text-slate-900">O que o M80 guarda</h2>
          <p>
            Para atualizar o extrato, o M80 guarda a sessão da conta na companhia (e, quando o
            usuário a informa no portal, a senha da companhia). Tudo isso fica criptografado no
            servidor e é usado só para ler saldo e extrato da própria conta. A senha nunca é
            mostrada de volta ao cliente.
          </p>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="text-base font-semibold text-slate-900">Compartilhamento</h2>
          <p>
            Os dados não são vendidos nem compartilhados com terceiros, não são usados para
            publicidade e não são usados para decidir crédito. Eles só trafegam entre o
            navegador do usuário, o M80 e a própria companhia aérea da conta.
          </p>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="text-base font-semibold text-slate-900">Como remover</h2>
          <p>
            A extensão pode ser removida a qualquer momento pela página de extensões do
            navegador. Para desconectar uma conta ou apagar os dados dela no M80, fale com a
            equipe que liberou o seu acesso ao portal.
          </p>
        </section>
      </article>
    </div>
  )
}
