import { SelectableValue } from '@grafana/data';

/** Minimal shape shared by every PRTG hierarchy DTO ({id, name, ...}). */
export interface HierarchyItem {
  id: string;
  name: string;
}

/** Maps a list of hierarchy DTOs to react-select options. */
export function toSelectableValues(items: HierarchyItem[]): Array<SelectableValue<string>> {
  return items.map((item) => ({ value: item.id, label: item.name }));
}

/**
 * Builds the option representing the currently selected id, preferring the
 * (possibly stale) display name saved on the query so a saved selection's
 * label shows before the async option list has loaded.
 */
export function toSelectedValue(
  id: string | undefined,
  name: string | undefined,
  options: Array<SelectableValue<string>>
): SelectableValue<string> | null {
  if (!id) {
    return null;
  }
  const label = name || options.find((option) => option.value === id)?.label || id;
  return { value: id, label };
}
