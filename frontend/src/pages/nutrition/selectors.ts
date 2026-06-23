import type {
  HydrationMode,
  NutritionDay,
  NutritionTargetsInput,
} from '@/lib/api'
import {
  MACRO_COLORS,
  MEAL_COLORS,
  MEAL_LABELS,
  MEAL_ORDER,
} from '@/pages/nutrition/constants'
import { parseRequiredDecimalOrNull } from '@/pages/nutrition/format'
import type {
  NutritionMacroSlice,
  NutritionMealStat,
  NutritionTargetsForm,
} from '@/pages/nutrition/types'

export function filterNutritionDayByMeal(day: NutritionDay, mealType: string): NutritionDay | null {
  if (!mealType) return day

  const meals = day.meals.filter(meal => meal.meal_type === mealType)
  if (meals.length === 0) return null

  const totals = meals.flatMap(meal => meal.items).reduce((sum, item) => ({
    calories: sum.calories + item.calories,
    protein: sum.protein + (item.macros?.protein ?? 0),
    carbs: sum.carbs + (item.macros?.carbs ?? 0),
    fat: sum.fat + (item.macros?.fat ?? 0),
    fiber: sum.fiber + (item.macros?.fiber ?? 0),
  }), { calories: 0, protein: 0, carbs: 0, fat: 0, fiber: 0 })

  return { ...day, ...totals, meals }
}

export function buildNutritionMacroSlices(data: NutritionDay[]): NutritionMacroSlice[] {
  if (data.length === 0) return []

  const averages = data.reduce((sum, day) => ({
    protein: sum.protein + day.protein,
    fat: sum.fat + day.fat,
    carbs: sum.carbs + day.carbs,
  }), { protein: 0, fat: 0, carbs: 0 })

  return [
    { name: 'Белки', value: averages.protein / data.length * 4, color: MACRO_COLORS.protein },
    { name: 'Жиры', value: averages.fat / data.length * 9, color: MACRO_COLORS.fat },
    { name: 'Углеводы', value: averages.carbs / data.length * 4, color: MACRO_COLORS.carbs },
  ].filter(macro => macro.value > 0)
}

export function buildNutritionMealStats(data: NutritionDay[]): NutritionMealStat[] {
  return MEAL_ORDER.map((mealType) => {
    const dailyCalories = data.map(day => {
      const meal = day.meals.find(entry => entry.meal_type === mealType)
      return meal?.items.reduce((sum, item) => sum + item.calories, 0) ?? 0
    })
    const presentCalories = dailyCalories.filter(calories => calories > 0)
    const totalCalories = presentCalories.reduce((sum, calories) => sum + calories, 0)

    return {
      key: mealType,
      name: MEAL_LABELS[mealType],
      color: MEAL_COLORS[mealType],
      totalCalories: Math.round(totalCalories),
      daysPresent: presentCalories.length,
      avgCaloriesWhenPresent: presentCalories.length > 0
        ? Math.round(totalCalories / presentCalories.length)
        : 0,
      avgCaloriesPerTrackedDay: data.length > 0 ? Math.round(totalCalories / data.length) : 0,
    }
  }).filter(stat => stat.totalCalories > 0)
}

export function buildNutritionTargetsPayload(
  form: NutritionTargetsForm,
  hydrationMode: HydrationMode,
): NutritionTargetsInput {
  return {
    target_weight_kg: parseRequiredDecimalOrNull('Целевой вес', form.targetWeightKg, { min: 20, max: 400 }),
    target_calories: parseRequiredDecimalOrNull('Калории', form.targetCalories, { min: 500, max: 10000 }),
    target_protein_g: parseRequiredDecimalOrNull('Белки', form.targetProteinG, { min: 1, max: 1000 }),
    target_carbs_g: parseRequiredDecimalOrNull('Углеводы', form.targetCarbsG, { min: 1, max: 1500 }),
    target_fat_g: parseRequiredDecimalOrNull('Жиры', form.targetFatG, { min: 1, max: 1000 }),
    target_water_ml: parseRequiredDecimalOrNull('Вода', form.targetWaterMl, { min: 100, max: 15000 }),
    hydration_mode: hydrationMode,
  }
}
