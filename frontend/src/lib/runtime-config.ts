declare global {
  interface Window {
    __APP_CONFIG__?: {
      SENTRY_FRONTEND_DSN?: string
      SENTRY_ENVIRONMENT?: string
      SENTRY_RELEASE?: string
      SENTRY_FRONTEND_TRACES_SAMPLE_RATE?: string
    }
  }
}

function readString(value: string | undefined): string {
  return value?.trim() ?? ''
}

function readNumber(value: string | undefined, fallback = 0): number {
  const parsed = Number(readString(value))
  return Number.isFinite(parsed) ? parsed : fallback
}

const config = window.__APP_CONFIG__ ?? {}

export const runtimeConfig = {
  sentryFrontendDsn: readString(config.SENTRY_FRONTEND_DSN),
  sentryEnvironment: readString(config.SENTRY_ENVIRONMENT),
  sentryRelease: readString(config.SENTRY_RELEASE),
  sentryFrontendTracesSampleRate: readNumber(config.SENTRY_FRONTEND_TRACES_SAMPLE_RATE, 0),
}

export type RuntimeConfig = typeof runtimeConfig
