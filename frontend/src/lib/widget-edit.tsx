import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'

interface WidgetEditContextValue {
  editingWidgets: boolean
  setEditingWidgets: (editing: boolean) => void
  toggleWidgetEditing: () => void
  canResetWidgets: boolean
  resetWidgets: () => void
  registerWidgetReset: (handler: () => void) => () => void
}

const WidgetEditContext = createContext<WidgetEditContextValue | null>(null)

export function WidgetEditProvider({ children }: { children: ReactNode }) {
  const [editingWidgets, setEditingWidgets] = useState(false)
  const [canResetWidgets, setCanResetWidgets] = useState(false)
  const resetHandlerRef = useRef<(() => void) | null>(null)

  const resetWidgets = useCallback(() => {
    resetHandlerRef.current?.()
  }, [])

  const registerWidgetReset = useCallback((handler: () => void) => {
    resetHandlerRef.current = handler
    setCanResetWidgets(true)

    return () => {
      if (resetHandlerRef.current !== handler) return
      resetHandlerRef.current = null
      setCanResetWidgets(false)
    }
  }, [])

  const value = useMemo<WidgetEditContextValue>(() => ({
    editingWidgets,
    setEditingWidgets,
    toggleWidgetEditing: () => setEditingWidgets(current => !current),
    canResetWidgets,
    resetWidgets,
    registerWidgetReset,
  }), [canResetWidgets, editingWidgets, registerWidgetReset, resetWidgets])

  return (
    <WidgetEditContext.Provider value={value}>
      {children}
    </WidgetEditContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useWidgetEdit() {
  const context = useContext(WidgetEditContext)
  if (!context) throw new Error('useWidgetEdit must be used inside WidgetEditProvider')
  return context
}
