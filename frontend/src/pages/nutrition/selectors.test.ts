import { describe, expect, it } from 'vitest'
import type { NutritionDay } from '@/lib/api'
import {
  buildNutritionMacroSlices,
  buildNutritionMealStats,
  buildNutritionTargetsPayload,
  filterNutritionDayByMeal,
} from '@/pages/nutrition/selectors'

const DAY: NutritionDay = {
  date: '2026-06-19',
  calories: 900,
  protein: 55,
  carbs: 95,
  fat: 32,
  fiber: 12,
  water_ml: 1400,
  hydration_ml: 1650,
  counted_drinks_ml: 250,
  other_drinks_ml: 0,
  beverages: [],
  meals: [
    {
      meal_type: 'breakfast',
      items: [{
        food_name: 'Каша',
        serving: '1 порция',
        calories: 300,
        macros: { protein: 10, carbs: 48, fat: 8, fiber: 6 },
      }],
    },
    {
      meal_type: 'lunch',
      items: [{
        food_name: 'Обед',
        serving: '1 порция',
        calories: 600,
        macros: { protein: 45, carbs: 47, fat: 24, fiber: 6 },
      }],
    },
  ],
}

describe('nutrition selectors', () => {
  it('filters a day and recalculates meal totals', () => {
    const filtered = filterNutritionDayByMeal(DAY, 'breakfast')

    expect(filtered).toMatchObject({
      calories: 300,
      protein: 10,
      carbs: 48,
      fat: 8,
      fiber: 6,
      hydration_ml: 1650,
    })
    expect(filtered?.meals).toHaveLength(1)
    expect(filterNutritionDayByMeal(DAY, 'dinner')).toBeNull()
  })

  it('builds macro calories from daily averages', () => {
    expect(buildNutritionMacroSlices([DAY])).toEqual([
      expect.objectContaining({ name: 'Белки', value: 220 }),
      expect.objectContaining({ name: 'Жиры', value: 288 }),
      expect.objectContaining({ name: 'Углеводы', value: 380 }),
    ])
  })

  it('summarizes meal presence and calories', () => {
    expect(buildNutritionMealStats([DAY])).toEqual([
      expect.objectContaining({
        key: 'breakfast',
        totalCalories: 300,
        daysPresent: 1,
        avgCaloriesWhenPresent: 300,
        avgCaloriesPerTrackedDay: 300,
      }),
      expect.objectContaining({
        key: 'lunch',
        totalCalories: 600,
        daysPresent: 1,
      }),
    ])
  })

  it('uses the current form when saving hydration mode', () => {
    expect(buildNutritionTargetsPayload({
      targetWeightKg: '78,5',
      targetCalories: '2400',
      targetProteinG: '160',
      targetCarbsG: '250',
      targetFatG: '70',
      targetWaterMl: '2500',
    }, 'flexible')).toEqual({
      target_weight_kg: 78.5,
      target_calories: 2400,
      target_protein_g: 160,
      target_carbs_g: 250,
      target_fat_g: 70,
      target_water_ml: 2500,
      hydration_mode: 'flexible',
    })
  })

  it('rejects implausible target values', () => {
    expect(() => buildNutritionTargetsPayload({
      targetWeightKg: '',
      targetCalories: '-100',
      targetProteinG: '',
      targetCarbsG: '',
      targetFatG: '',
      targetWaterMl: '',
    }, 'strict')).toThrow('не меньше 500')
  })
})
