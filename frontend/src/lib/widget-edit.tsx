import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

interface WidgetEditContextValue {
  editingWidgets: boolean
  setEditingWidgets: (editing: boolean) => void
  toggleWidgetEditing: () => void
}

const WidgetEditContext = createContext<WidgetEditContextValue | null>(null)

export function WidgetEditProvider({ children }: { children: ReactNode }) {
  const [editingWidgets, setEditingWidgets] = useState(false)

  const value = useMemo<WidgetEditContextValue>(() => ({
    editingWidgets,
    setEditingWidgets,
    toggleWidgetEditing: () => setEditingWidgets(current => !current),
  }), [editingWidgets])

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
