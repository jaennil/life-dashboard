import { useEffect, useMemo, useState } from 'react'
import { Download, Share, Smartphone, X } from 'lucide-react'
import { cn } from '@/lib/utils'

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

const DISMISS_KEY = 'pwa-install-prompt-dismissed'

function isStandalone() {
  return window.matchMedia('(display-mode: standalone)').matches || Boolean((window.navigator as Navigator & { standalone?: boolean }).standalone)
}

function isIOS() {
  const ua = window.navigator.userAgent.toLowerCase()
  return /iphone|ipad|ipod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
}

export function InstallPrompt() {
  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.localStorage.getItem(DISMISS_KEY) === '1'
  })
  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null)
  const [installed, setInstalled] = useState(() => {
    if (typeof window === 'undefined') return false
    return isStandalone()
  })
  const ios = useMemo(() => isIOS(), [])

  const mode = useMemo(() => {
    if (deferredPrompt) return 'prompt'
    if (ios) return 'ios'
    return 'generic'
  }, [deferredPrompt, ios])

  useEffect(() => {
    if (dismissed || installed || ios) return

    const onBeforeInstallPrompt = (event: Event) => {
      event.preventDefault()
      setDeferredPrompt(event as BeforeInstallPromptEvent)
    }

    const onInstalled = () => {
      setInstalled(true)
      setDeferredPrompt(null)
    }

    window.addEventListener('beforeinstallprompt', onBeforeInstallPrompt)
    window.addEventListener('appinstalled', onInstalled)
    return () => {
      window.removeEventListener('beforeinstallprompt', onBeforeInstallPrompt)
      window.removeEventListener('appinstalled', onInstalled)
    }
  }, [dismissed, installed, ios])

  const visible = !dismissed && !installed && (ios || deferredPrompt !== null)

  if (!visible) return null

  async function handleInstall() {
    if (!deferredPrompt) return
    await deferredPrompt.prompt()
    const choice = await deferredPrompt.userChoice
    if (choice.outcome === 'accepted') {
      setInstalled(true)
    }
    setDeferredPrompt(null)
  }

  function dismiss() {
    localStorage.setItem(DISMISS_KEY, '1')
    setDismissed(true)
  }

  return (
    <div
      className={cn(
        'fixed z-50 rounded-3xl border bg-card/95 p-4 shadow-2xl backdrop-blur',
        'left-4 right-4 bottom-24 lg:left-auto lg:right-6 lg:bottom-6 lg:max-w-sm'
      )}
    >
      <div className="flex items-start gap-3">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary">
          {mode === 'ios' ? <Share className="h-5 w-5" /> : <Download className="h-5 w-5" />}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-foreground">Установить Life Dashboard</p>
          {mode === 'ios' ? (
            <p className="mt-1 text-sm text-muted-foreground">
              На iPhone открой меню <span className="font-medium text-foreground">Поделиться</span> в Safari и выбери
              <span className="font-medium text-foreground"> «На экран Домой»</span>.
            </p>
          ) : mode === 'prompt' ? (
            <p className="mt-1 text-sm text-muted-foreground">
              Установи приложение на главный экран, чтобы запускать его в fullscreen и быстрее открывать нужные вкладки.
            </p>
          ) : (
            <p className="mt-1 text-sm text-muted-foreground">
              Открой сайт в мобильном браузере и добавь его на главный экран как приложение.
            </p>
          )}
          <div className="mt-3 flex flex-wrap items-center gap-2">
            {mode === 'prompt' ? (
              <button
                onClick={() => { void handleInstall() }}
                className="inline-flex items-center gap-2 rounded-2xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
              >
                <Smartphone className="h-4 w-4" />
                Установить
              </button>
            ) : null}
            <button
              onClick={dismiss}
              className="inline-flex items-center gap-2 rounded-2xl border px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              Позже
            </button>
          </div>
        </div>
        <button
          onClick={dismiss}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
          aria-label="Скрыть подсказку установки"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}
