export type NutritionRawFilters = Record<string, string | undefined>

export type OpenNutritionRaw = (filters?: NutritionRawFilters) => void

export type NutritionMacroSlice = {
  name: string
  value: number
  color: string
}

export type NutritionMealStat = {
  key: string
  name: string
  color: string
  totalCalories: number
  daysPresent: number
  avgCaloriesWhenPresent: number
  avgCaloriesPerTrackedDay: number
}

export type NutritionTargetsForm = {
  targetWeightKg: string
  targetCalories: string
  targetProteinG: string
  targetCarbsG: string
  targetFatG: string
  targetWaterMl: string
}
