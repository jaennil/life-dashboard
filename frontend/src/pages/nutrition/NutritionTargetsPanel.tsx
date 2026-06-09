import { useState } from 'react'
import { ExpandablePanel } from '@/components/ExpandablePanel'
import { InfoTooltip } from '@/components/InfoTooltip'
import { cn } from '@/lib/utils'
import type { HydrationMode, NutritionTargets } from '@/lib/api'
import { HYDRATION_MODE_NOTES } from '@/pages/nutrition/constants'
import {
  fmtDate,
  fmtOptionalNumber,
  fmtSyncTime,
  fmtTargetDelta,
  fmtWeight,
  hydrationModeLabel,
} from '@/pages/nutrition/format'
import type { NutritionTargetsForm } from '@/pages/nutrition/types'

export function NutritionTargetsPanel({
  targets,
  targetsForm,
  hydrationMode,
  savingTargets,
  targetsError,
  targetsNotice,
  onTargetsFieldChange,
  onSaveTargets,
}: {
  targets: NutritionTargets | undefined
  targetsForm: NutritionTargetsForm
  hydrationMode: HydrationMode
  savingTargets: boolean
  targetsError: string
  targetsNotice: string
  onTargetsFieldChange: (field: keyof NutritionTargetsForm, value: string) => void
  onSaveTargets: (clear?: boolean) => void | Promise<void>
}) {
  const [open, setOpen] = useState(false)

  return (
    <ExpandablePanel
      title="Цели питания и ручные настройки"
      description={undefined}
      open={open}
      onToggle={() => setOpen(current => !current)}
      summary={(
        <>
          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
            Цель: {fmtOptionalNumber(targets?.target_calories, ' ккал')}
          </span>
          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
            Б/Ж/У: {fmtOptionalNumber(targets?.target_protein_g, ' г')} · {fmtOptionalNumber(targets?.target_fat_g, ' г')} · {fmtOptionalNumber(targets?.target_carbs_g, ' г')}
          </span>
          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
            Вода: {fmtOptionalNumber(targets?.target_water_ml, ' мл')}
          </span>
          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
            Гидратация: {hydrationModeLabel(hydrationMode)}
          </span>
          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
            Вес: {fmtWeight(targets?.current_weight_kg)} → {fmtWeight(targets?.target_weight_kg)}
          </span>
          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
            До цели: {fmtTargetDelta(targets?.current_weight_kg, targets?.target_weight_kg)}
          </span>
          {targets?.source ? (
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 uppercase tracking-wide">
              {targets.source}
            </span>
          ) : null}
        </>
      )}
    >
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-7">
          <div className="rounded-xl bg-muted/40 p-3">
            <p className="text-[10px] text-muted-foreground">Текущий вес</p>
            <p className="mt-1 text-lg font-bold text-foreground">{fmtWeight(targets?.current_weight_kg)}</p>
            {targets?.current_weight_date && <p className="mt-1 text-[10px] text-muted-foreground">{fmtDate(targets.current_weight_date)}</p>}
          </div>
          <div className="rounded-xl bg-muted/40 p-3">
            <p className="text-[10px] text-muted-foreground">Целевой вес</p>
            <p className="mt-1 text-lg font-bold text-foreground">{fmtWeight(targets?.target_weight_kg)}</p>
          </div>
          <div className="rounded-xl bg-muted/40 p-3">
            <p className="text-[10px] text-muted-foreground">До цели</p>
            <p className="mt-1 text-lg font-bold text-foreground">{fmtTargetDelta(targets?.current_weight_kg, targets?.target_weight_kg)}</p>
          </div>
          <div className="rounded-xl bg-muted/40 p-3">
            <p className="text-[10px] text-muted-foreground">Рост</p>
            <p className="mt-1 text-lg font-bold text-foreground">{fmtOptionalNumber(targets?.height_cm, ' см')}</p>
          </div>
          <div className="rounded-xl bg-muted/40 p-3">
            <p className="text-[10px] text-muted-foreground">Цель ккал</p>
            <p className="mt-1 text-lg font-bold text-foreground">{fmtOptionalNumber(targets?.target_calories, ' ккал')}</p>
          </div>
          <div className="rounded-xl bg-cyan-500/10 p-3">
            <p className="text-[10px] text-cyan-200/80">Цель воды</p>
            <p className="mt-1 text-lg font-bold text-cyan-100">{fmtOptionalNumber(targets?.target_water_ml, ' мл')}</p>
          </div>
          <div className={cn('rounded-xl p-3', hydrationMode === 'flexible' ? 'bg-emerald-500/10' : 'bg-cyan-500/10')}>
            <p className="inline-flex items-center gap-1 text-[10px] text-muted-foreground">
              Режим гидратации
              <InfoTooltip text={HYDRATION_MODE_NOTES[hydrationMode]} />
            </p>
            <p className="mt-1 text-lg font-bold text-foreground">{hydrationModeLabel(hydrationMode)}</p>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div className="rounded-xl bg-blue-500/10 p-3">
            <p className="text-[10px] text-blue-300">Белки</p>
            <p className="mt-1 text-base font-bold text-blue-200">{fmtOptionalNumber(targets?.target_protein_g, ' г')}</p>
          </div>
          <div className="rounded-xl bg-orange-500/10 p-3">
            <p className="text-[10px] text-orange-300">Жиры</p>
            <p className="mt-1 text-base font-bold text-orange-200">{fmtOptionalNumber(targets?.target_fat_g, ' г')}</p>
          </div>
          <div className="rounded-xl bg-emerald-500/10 p-3">
            <p className="text-[10px] text-emerald-300">Углеводы</p>
            <p className="mt-1 text-base font-bold text-emerald-200">{fmtOptionalNumber(targets?.target_carbs_g, ' г')}</p>
          </div>
        </div>

        <div className="rounded-xl border border-dashed bg-muted/20 p-4">
          <div className="mb-4">
            <h3 className="inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-foreground">
              Ручные цели
              <InfoTooltip text="Заполняй только то, чего не хватает в FatSecret, или то, что хочешь переопределить вручную." />
            </h3>
            {targets?.manual?.updated_at ? (
              <p className="mt-1 text-[11px] text-muted-foreground">Последнее ручное обновление: {fmtSyncTime(targets.manual.updated_at)}</p>
            ) : null}
            {targets?.synced_at ? (
              <p className="mt-1 text-[11px] text-muted-foreground">Последнее обновление целей: {fmtSyncTime(targets.synced_at)}</p>
            ) : null}
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-6">
            <label className="flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">Целевой вес, кг</span>
              <input
                type="text"
                inputMode="decimal"
                value={targetsForm.targetWeightKg}
                onChange={e => onTargetsFieldChange('targetWeightKg', e.target.value)}
                placeholder="например, 78"
                className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">Калории, ккал</span>
              <input
                type="text"
                inputMode="decimal"
                value={targetsForm.targetCalories}
                onChange={e => onTargetsFieldChange('targetCalories', e.target.value)}
                placeholder="например, 2400"
                className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">Белки, г</span>
              <input
                type="text"
                inputMode="decimal"
                value={targetsForm.targetProteinG}
                onChange={e => onTargetsFieldChange('targetProteinG', e.target.value)}
                placeholder="например, 160"
                className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">Жиры, г</span>
              <input
                type="text"
                inputMode="decimal"
                value={targetsForm.targetFatG}
                onChange={e => onTargetsFieldChange('targetFatG', e.target.value)}
                placeholder="например, 70"
                className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">Углеводы, г</span>
              <input
                type="text"
                inputMode="decimal"
                value={targetsForm.targetCarbsG}
                onChange={e => onTargetsFieldChange('targetCarbsG', e.target.value)}
                placeholder="например, 250"
                className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">Вода, мл</span>
              <input
                type="text"
                inputMode="decimal"
                value={targetsForm.targetWaterMl}
                onChange={e => onTargetsFieldChange('targetWaterMl', e.target.value)}
                placeholder="например, 2500"
                className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
          </div>

          {(targetsError || targetsNotice) ? (
            <p className={cn('mt-3 text-xs', targetsError ? 'text-rose-400' : 'text-emerald-400')}>
              {targetsError || targetsNotice}
            </p>
          ) : null}

          <div className="mt-4 flex flex-wrap gap-2">
            <button
              onClick={() => void onSaveTargets(false)}
              disabled={savingTargets}
              className="rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {savingTargets ? 'Сохраняю...' : 'Сохранить ручные цели'}
            </button>
            <button
              onClick={() => void onSaveTargets(true)}
              disabled={savingTargets}
              className="rounded-lg border px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent disabled:opacity-50"
            >
              Очистить ручные цели
            </button>
          </div>
        </div>

        {targets?.api_notes?.map(note => (
          <p key={note} className="rounded-xl border border-dashed px-3 py-2 text-xs text-muted-foreground">{note}</p>
        ))}
      </div>
    </ExpandablePanel>
  )
}
