"use client"

import Image from "next/image"
import { useState, useTransition } from "react"
import { AlertTriangle, CheckCircle2, FlaskConical } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { promoSourceLabel } from "@/lib/promo"
import { testPromoRecipeAction } from "@/actions/promo"
import type { PromoRecipe } from "@/schemas/promo"

interface TestRecipePanelProps {
  recipe: PromoRecipe
  isOpen: boolean
}

type TestResult = {
  resolved?: {
    cpf_field: string
    checkbox: boolean
    submit: string
    source: string
  } | null
  error?: string | null
  screenshot_base64?: string | null
}

// The dry run, from the panel. It opens the page, tries to find the form and takes a
// screenshot - and submits nothing, which is what makes it safe to use against a live
// campaign. The recipe goes over as it is currently typed, saved or not, so a fix can
// be tried before it is committed to.
export function TestRecipePanel({ recipe, isOpen }: TestRecipePanelProps) {
  const [url, setUrl] = useState("")
  const [result, setResult] = useState<TestResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isPending, startTransition] = useTransition()

  function handleTest() {
    setError(null)
    setResult(null)
    startTransition(async () => {
      const response = await testPromoRecipeAction({
        url,
        cpf_selector: recipe.cpf_selector ?? "",
        checkbox_selector: recipe.checkbox_selector ?? "",
        submit_selector: recipe.submit_selector ?? "",
        success_pattern: recipe.success_pattern ?? "",
        failure_pattern: recipe.failure_pattern ?? "",
        wait_seconds: String(recipe.wait_seconds),
      })

      if (response.error) {
        setError(response.error)
        return
      }
      setResult(response.result ?? null)
    })
  }

  return (
    <div
      className={cn(
        "grid transition-all duration-300 ease-in-out",
        isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
    >
      <div className="overflow-hidden">
        <div className="flex flex-col gap-4 rounded-b-md border border-t-0 border-slate-200 p-4">
          <div className="flex flex-col gap-1">
            <p className="text-sm font-medium text-slate-800">Testar em uma página</p>
            <p className="text-xs text-slate-500">
              Abre o link, procura o formulário e devolve um print.{" "}
              <span className="font-medium">Nada é enviado.</span>
            </p>
          </div>

          <div className="flex gap-2">
            <Input
              value={url}
              onChange={(event) => setUrl(event.target.value)}
              placeholder={`https://${recipe.domain}/promocao`}
              className="bg-slate-200"
              disabled={isPending}
            />
            <Button
              type="button"
              onClick={handleTest}
              disabled={isPending || url.trim() === ""}
              className="bg-slate-800 hover:bg-slate-900"
            >
              {isPending ? (
                <Spinner className="h-4 w-4" />
              ) : (
                <span className="flex items-center gap-2 text-sm">
                  <FlaskConical className="h-4 w-4" />
                  Testar
                </span>
              )}
            </Button>
          </div>

          {isPending && (
            <p className="text-xs text-slate-500">
              Abrindo a página num navegador de verdade — costuma levar uns 15 segundos.
            </p>
          )}

          {error && <p className="text-sm text-red-600">{error}</p>}

          {result && (
            <div className="flex flex-col gap-3">
              {result.resolved ? (
                <div className="flex flex-col gap-1 rounded-md bg-emerald-50 p-3 text-sm text-emerald-900">
                  <span className="flex items-center gap-2 font-medium">
                    <CheckCircle2 className="h-4 w-4" />
                    Formulário localizado {promoSourceLabel(result.resolved.source)}
                  </span>
                  <span className="text-xs">
                    Campo de CPF: <code>{result.resolved.cpf_field || "sem seletor"}</code>
                  </span>
                  <span className="text-xs">
                    Botão: <code>{result.resolved.submit}</code>
                  </span>
                  <span className="text-xs">
                    Aceite das regras:{" "}
                    {result.resolved.checkbox ? "encontrado" : "não encontrado"}
                  </span>
                </div>
              ) : (
                <div className="flex flex-col gap-1 rounded-md bg-amber-50 p-3 text-sm text-amber-900">
                  <span className="flex items-center gap-2 font-medium">
                    <AlertTriangle className="h-4 w-4" />
                    Nenhum formulário de cadastro encontrado
                  </span>
                  <span className="text-xs">
                    O bot recusaria essa página em vez de arriscar. Confira o print
                    abaixo e ajuste os seletores.
                  </span>
                  {result.error && (
                    <span className="text-xs opacity-80">{result.error}</span>
                  )}
                </div>
              )}

              {result.screenshot_base64 && (
                <div className="overflow-hidden rounded-md border border-slate-200">
                  <Image
                    src={`data:image/png;base64,${result.screenshot_base64}`}
                    alt="Página como o bot a viu"
                    width={1440}
                    height={900}
                    unoptimized
                    className="h-auto w-full"
                  />
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
