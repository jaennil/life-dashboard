import * as Sentry from '@sentry/react'
import { runtimeConfig } from '@/lib/runtime-config'

const environment = runtimeConfig.sentryEnvironment || (import.meta.env.PROD ? 'production' : 'development')

export const sentryEnabled = runtimeConfig.sentryFrontendDsn !== ''

export function initSentry() {
  if (!sentryEnabled) {
    return
  }

  Sentry.init({
    dsn: runtimeConfig.sentryFrontendDsn,
    environment,
    release: runtimeConfig.sentryRelease || undefined,
    tracesSampleRate: runtimeConfig.sentryFrontendTracesSampleRate,
  })
}

export function setSentryUser(user: { id: string; username: string } | null) {
  if (!sentryEnabled) {
    return
  }

  Sentry.setUser(user ? { id: user.id, username: user.username } : null)
}

export function captureAPIFailure(input: {
  path: string
  method: string
  error: unknown
  statusCode?: number
  responseBody?: string
}) {
  if (!sentryEnabled) {
    return
  }

  Sentry.withScope((scope) => {
    scope.setTag('source', 'api')
    scope.setTag('method', input.method)
    scope.setTag('path', input.path)
    if (input.statusCode != null) {
      scope.setTag('status_code', String(input.statusCode))
    }
    scope.setContext('api_request', {
      path: input.path,
      method: input.method,
      status_code: input.statusCode,
      response_body: input.responseBody?.slice(0, 1000),
    })

    if (input.error instanceof Error) {
      Sentry.captureException(input.error)
      return
    }

    Sentry.captureMessage(`api request failed: ${input.method} ${input.path}`)
  })
}
