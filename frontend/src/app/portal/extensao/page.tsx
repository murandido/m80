import type { Metadata } from "next"
import Image from "next/image"
import Link from "next/link"
import { ArrowLeft, ExternalLink, Laptop, ShieldCheck } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ExtensionStatus } from "@/components/portal/extension-status"
import { env } from "@/lib/env"

export const metadata: Metadata = {
  title: "Extensão M80 Extrato · M80",
  description: "Como instalar a extensão que conecta as suas contas Azul e Smiles.",
}

// The store link is read per request from the server's env (the item is unlisted).
export const dynamic = "force-dynamic"

interface Step {
  title: string
  body: React.ReactNode
  // Screenshot under public/portal/extensao, with its real size so the layout does not jump.
  image?: { src: string; alt: string; width: number; height: number }
}

const INSTALL_STEPS: Step[] = [
  {
    title: "Abra a página da extensão",
    body: "Use o botão “Abrir na Chrome Web Store” acima. A página abre numa nova aba.",
  },
  {
    title: "Clique em “Usar no Chrome”",
    body: "É o botão azul no canto direito da página. Em outros navegadores ele pode se chamar “Obter” ou “Adicionar”.",
    image: {
      src: "/portal/extensao/loja.png",
      alt: "Página da M80 Extrato na Chrome Web Store com o botão Usar no Chrome",
      width: 1110,
      height: 630,
    },
  },
  {
    title: "Confirme em “Adicionar extensão”",
    body: "O navegador mostra o que a extensão pode acessar: o site do M80 e as páginas de login da Azul e da Smiles. Ela não lê nenhum outro site. Se o seu Chrome estiver em inglês, o botão se chama “Add extension”.",
    image: {
      src: "/portal/extensao/adicionar.png",
      alt: "Diálogo do Chrome pedindo para confirmar a instalação da M80 Extrato",
      width: 460,
      height: 240,
    },
  },
  {
    title: "Recarregue esta página",
    body: "Aperte F5. O aviso no topo desta página fica verde quando a extensão está pronta.",
    image: {
      src: "/portal/extensao/instalada.png",
      alt: "Aviso verde dizendo que a extensão já está instalada neste navegador",
      width: 658,
      height: 82,
    },
  },
]

const USE_STEPS: Step[] = [
  {
    title: "Smiles: clique em Conectar e escolha onde receber o código",
    body: "Na página da conta, clique em Conectar e escolha SMS ou e-mail. Em alguns segundos a Smiles envia um código de seis dígitos. Digite o código e clique em Confirmar código. Confira os dígitos antes de enviar, porque a Smiles limita as tentativas.",
  },
  {
    title: "Azul: clique em Pelo navegador",
    body: "Use quando o Conectar normal não funcionar. Abre o site da Azul numa nova aba. Clique em Entrar, faça o login com o seu CPF e a sua senha da Azul, e volte para o M80. A aba fecha sozinha quando a conexão termina.",
  },
]

function StepList({ steps, startAt = 1 }: { steps: Step[]; startAt?: number }) {
  return (
    <ol className="flex flex-col gap-5">
      {steps.map((step, index) => (
        <li key={step.title} className="flex gap-3">
          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-sky-100 text-sm font-semibold text-sky-800">
            {startAt + index}
          </span>
          <div className="flex min-w-0 flex-col gap-2">
            <h3 className="text-sm font-semibold text-slate-800">{step.title}</h3>
            <p className="text-sm text-slate-600">{step.body}</p>
            {step.image && (
              <Image
                src={step.image.src}
                alt={step.image.alt}
                width={step.image.width}
                height={step.image.height}
                className="h-auto w-auto max-w-full rounded-md border border-slate-200"
              />
            )}
          </div>
        </li>
      ))}
    </ol>
  )
}

export default function PortalExtensionPage() {
  const storeUrl = env.CLIENT_EXTENSION_STORE_URL || null

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-6 px-4 py-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2 text-slate-500">
          <Link href="/portal">
            <ArrowLeft className="mr-1 h-4 w-4" />
            Suas contas
          </Link>
        </Button>
        <h1 className="text-lg font-semibold text-slate-800">Extensão M80 Extrato</h1>
        <p className="mt-1 text-sm text-slate-600">
          Para proteger as contas, a Smiles só envia o código de verificação para um navegador
          de verdade, e o login da Azul às vezes também precisa ser feito por você. A extensão
          M80 Extrato faz essa ponte: ela abre a página da companhia no seu navegador e devolve
          a conexão para o M80. Você instala uma vez e ela fica pronta para sempre.
        </p>
      </div>

      <ExtensionStatus />

      <div className="flex flex-col gap-3 rounded-md border border-slate-200 p-4">
        {storeUrl ? (
          <Button asChild className="self-start">
            <a href={storeUrl} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="mr-1 h-4 w-4" />
              Abrir na Chrome Web Store
            </a>
          </Button>
        ) : (
          <p className="text-sm text-amber-800">
            A extensão estará disponível em breve. Enquanto isso, fale com a equipe M80 para
            conectar a sua conta.
          </p>
        )}
        <p className="flex items-start gap-2 text-xs text-slate-500">
          <Laptop className="mt-0.5 h-4 w-4 shrink-0" />
          Funciona no computador, no Google Chrome, Microsoft Edge, Brave ou Opera. Não funciona
          no celular nem no Safari.
        </p>
        <p className="flex items-start gap-2 text-xs text-slate-500">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" />
          A extensão não guarda a sua senha e não lê outros sites. Ela só age quando você clica em
          Conectar ou Pelo navegador aqui no M80.
        </p>
      </div>

      <section className="flex flex-col gap-4">
        <h2 className="text-base font-semibold text-slate-800">Como instalar</h2>
        <StepList steps={INSTALL_STEPS} />
      </section>

      <section className="flex flex-col gap-4">
        <h2 className="text-base font-semibold text-slate-800">Como conectar a conta</h2>
        <StepList steps={USE_STEPS} startAt={INSTALL_STEPS.length + 1} />
      </section>
    </div>
  )
}
