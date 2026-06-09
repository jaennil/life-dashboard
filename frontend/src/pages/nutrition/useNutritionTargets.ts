import { useCallback, useEffect, useState } from 'react'
import { api, type HydrationMode, type NutritionTargets, type NutritionTargetsInput } from '@/lib/api'
import {
  hydrationModeLabel,
  numberInputValue,
  parseRequiredDecimalOrNull,
} from '@/pages/nutrition/format'
import type { NutritionTargetsForm } from '@/pages/nutrition/types'

const EMPTY_TARGETS_FORM: NutritionTargetsForm = {
  targetWeightKg: '',
  targetCalories: '',
  targetProteinG: '',
  targetCarbsG: '',
  targetFatG: '',
  targetWaterMl: '',
}

export function useNutritionTargets({
  targets,
  reloadNutritionData,
  onHydrationModeChangeStart,
}: {
  targets: NutritionTargets | undefined
  reloadNutritionData: () => Promise<void>
  onHydrationModeChangeStart?: () => void
}) {
  const [savingTargets, setSavingTargets] = useState(false)
  const [targetsError, setTargetsError] = useState('')
  const [targetsNotice, setTargetsNotice] = useState('')
  const [hydrationMode, setHydrationMode] = useState<HydrationMode>('strict')
  const [targetsForm, setTargetsForm] = useState<NutritionTargetsForm>(EMPTY_TARGETS_FORM)

  useEffect(() => {
    const manual = targets?.manual
    setTargetsForm({
      targetWeightKg: numberInputValue(manual?.target_weight_kg),
      targetCalories: numberInputValue(manual?.target_calories),
      targetProteinG: numberInputValue(manual?.target_protein_g),
      targetCarbsG: numberInputValue(manual?.target_carbs_g),
      targetFatG: numberInputValue(manual?.target_fat_g),
      targetWaterMl: numberInputValue(manual?.target_water_ml),
    })
    setHydrationMode(targets?.hydration_mode ?? 'strict')
  }, [targets])

  const setTargetsField = useCallback((field: keyof NutritionTargetsForm, value: string) => {
    setTargetsForm(current => ({ ...current, [field]: value }))
  }, [])

  const buildSavedTargetsPayload = useCallback((nextMode: HydrationMode): NutritionTargetsInput => {
    const manual = targets?.manual
    return {
      target_weight_kg: manual?.target_weight_kg ?? null,
      target_calories: manual?.target_calories ?? null,
      target_protein_g: manual?.target_protein_g ?? null,
      target_carbs_g: manual?.target_carbs_g ?? null,
      target_fat_g: manual?.target_fat_g ?? null,
      target_water_ml: manual?.target_water_ml ?? null,
      hydration_mode: nextMode,
    }
  }, [targets])

  const handleSaveTargets = useCallback(async (clear = false) => {
    setSavingTargets(true)
    setTargetsError('')
    setTargetsNotice('')

    try {
      const payload: NutritionTargetsInput = clear ? {
        target_weight_kg: null,
        target_calories: null,
        target_protein_g: null,
        target_carbs_g: null,
        target_fat_g: null,
        target_water_ml: null,
        hydration_mode: null,
      } : {
        target_weight_kg: parseRequiredDecimalOrNull('Целевой вес', targetsForm.targetWeightKg),
        target_calories: parseRequiredDecimalOrNull('Калории', targetsForm.targetCalories),
        target_protein_g: parseRequiredDecimalOrNull('Белки', targetsForm.targetProteinG),
        target_carbs_g: parseRequiredDecimalOrNull('Углеводы', targetsForm.targetCarbsG),
        target_fat_g: parseRequiredDecimalOrNull('Жиры', targetsForm.targetFatG),
        target_water_ml: parseRequiredDecimalOrNull('Вода', targetsForm.targetWaterMl),
        hydration_mode: hydrationMode,
      }
      await api.saveNutritionTargets(payload)
      if (clear) {
        setTargetsForm(EMPTY_TARGETS_FORM)
        setHydrationMode('strict')
      }
      await reloadNutritionData()
      setTargetsNotice(clear ? 'Ручные цели очищены' : 'Ручные цели сохранены')
    } catch (error) {
      setTargetsError(error instanceof Error ? error.message : 'Не удалось сохранить ручные цели')
    } finally {
      setSavingTargets(false)
    }
  }, [hydrationMode, reloadNutritionData, targetsForm])

  const handleHydrationModeChange = useCallback(async (nextMode: HydrationMode) => {
    const previousMode = hydrationMode
    setHydrationMode(nextMode)
    setSavingTargets(true)
    setTargetsError('')
    setTargetsNotice('')
    onHydrationModeChangeStart?.()
    try {
      await api.saveNutritionTargets(buildSavedTargetsPayload(nextMode))
      await reloadNutritionData()
      setTargetsNotice(`Режим гидратации: ${hydrationModeLabel(nextMode).toLowerCase()}`)
    } catch (error) {
      setHydrationMode(previousMode)
      setTargetsError(error instanceof Error ? error.message : 'Не удалось сохранить режим гидратации')
    } finally {
      setSavingTargets(false)
    }
  }, [buildSavedTargetsPayload, hydrationMode, onHydrationModeChangeStart, reloadNutritionData])

  return {
    savingTargets,
    targetsError,
    targetsNotice,
    targetsForm,
    hydrationMode,
    setTargetsField,
    handleSaveTargets,
    handleHydrationModeChange,
  }
}
