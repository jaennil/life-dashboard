import { useCallback, useState } from 'react'
import { api, type HydrationBeverageType } from '@/lib/api'
import { hydrationBeverageLabel, parseRequiredDecimalOrNull } from '@/pages/nutrition/format'

export function useNutritionHydration({
  reloadNutritionData,
  targetDate,
  targetDateLabel,
}: {
  reloadNutritionData: () => Promise<void>
  targetDate?: string
  targetDateLabel: string
}) {
  const [savingWater, setSavingWater] = useState(false)
  const [waterError, setWaterError] = useState('')
  const [waterNotice, setWaterNotice] = useState('')
  const [customWaterInput, setCustomWaterInput] = useState('')
  const [customHydrationType, setCustomHydrationType] = useState<HydrationBeverageType>('tea')
  const [customHydrationInput, setCustomHydrationInput] = useState('')

  const clearHydrationMessages = useCallback(() => {
    setWaterError('')
    setWaterNotice('')
  }, [])

  const handleAddWater = useCallback(async (deltaMl: number) => {
    setSavingWater(true)
    clearHydrationMessages()
    try {
      const next = await api.saveNutritionWater({ date: targetDate, delta_ml: deltaMl })
      await reloadNutritionData()
      setWaterNotice(`Добавлено ${Math.round(deltaMl)} мл воды · в цель ${targetDateLabel} ${Math.round(next.hydration_ml)} мл`)
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось сохранить воду')
    } finally {
      setSavingWater(false)
    }
  }, [clearHydrationMessages, reloadNutritionData, targetDate, targetDateLabel])

  const handleSetWaterAbsolute = useCallback(async (nextWaterMl: number) => {
    setSavingWater(true)
    clearHydrationMessages()
    try {
      const next = await api.saveNutritionWater({ date: targetDate, water_ml: nextWaterMl })
      await reloadNutritionData()
      setWaterNotice(nextWaterMl === 0
        ? `Вода ${targetDateLabel} сброшена`
        : `Вода ${targetDateLabel}: ${Math.round(next.water_ml)} мл · в цель ${Math.round(next.hydration_ml)} мл`)
      if (nextWaterMl > 0) setCustomWaterInput('')
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось обновить воду')
    } finally {
      setSavingWater(false)
    }
  }, [clearHydrationMessages, reloadNutritionData, targetDate, targetDateLabel])

  const handleSubmitCustomWater = useCallback(async () => {
    try {
      const parsed = parseRequiredDecimalOrNull('Вода', customWaterInput, { min: 1, max: 15000 })
      if (parsed == null) {
        setWaterError('Введи количество воды в мл')
        return
      }
      await handleSetWaterAbsolute(parsed)
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось обновить воду')
    }
  }, [customWaterInput, handleSetWaterAbsolute])

  const handleAddHydration = useCallback(async (beverageType: HydrationBeverageType, deltaMl: number) => {
    setSavingWater(true)
    clearHydrationMessages()
    try {
      const next = await api.saveNutritionHydration({ date: targetDate, beverage_type: beverageType, delta_ml: deltaMl })
      await reloadNutritionData()
      setWaterNotice(`${hydrationBeverageLabel(beverageType)} +${Math.round(deltaMl)} мл · в цель ${targetDateLabel} ${Math.round(next.hydration_ml)} мл`)
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось сохранить напиток')
    } finally {
      setSavingWater(false)
    }
  }, [clearHydrationMessages, reloadNutritionData, targetDate, targetDateLabel])

  const handleSubmitCustomHydration = useCallback(async () => {
    try {
      const parsed = parseRequiredDecimalOrNull('Напиток', customHydrationInput, { min: 1, max: 15000 })
      if (parsed == null) {
        setWaterError('Введи количество напитка в мл')
        return
      }
      await handleAddHydration(customHydrationType, parsed)
      setCustomHydrationInput('')
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось сохранить напиток')
    }
  }, [customHydrationInput, customHydrationType, handleAddHydration])

  return {
    savingWater,
    waterError,
    waterNotice,
    customWaterInput,
    customHydrationType,
    customHydrationInput,
    setCustomWaterInput,
    setCustomHydrationType,
    setCustomHydrationInput,
    clearHydrationMessages,
    handleAddWater,
    handleSetWaterAbsolute,
    handleSubmitCustomWater,
    handleAddHydration,
    handleSubmitCustomHydration,
  }
}
