import { useEffect, useState } from 'react'
import { applyTheme, getPreferredTheme, type Theme } from '@/lib/theme'

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(getPreferredTheme)

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  const toggle = () => setTheme(t => t === 'dark' ? 'light' : 'dark')

  return { theme, toggle }
}
