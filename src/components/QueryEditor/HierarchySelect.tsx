import React, { useCallback, useMemo } from 'react';
import { SelectableValue } from '@grafana/data';
import { AsyncSelect } from '@grafana/ui';
import { HierarchyItem, toSelectedValue } from './toSelectableValue';
import { useResourceOptions } from './useResourceOptions';

interface Props {
  inputId?: string;
  ['aria-label']?: string;
  placeholder?: string;
  /** Selected id, e.g. `query.groupId`. */
  value?: string;
  /** Display-only saved label, e.g. `query.groupName` — shown before options load. */
  valueName?: string;
  onChange: (id: string | undefined, name: string | undefined) => void;
  /** Loads this select's option list; memoize with `useCallback` so it only changes with its real inputs. */
  fetcher: () => Promise<HierarchyItem[]>;
  /** Extra options prepended before the fetched ones, e.g. a synthetic "All channels" entry. */
  extraOptions?: HierarchyItem[];
  disabled?: boolean;
  width?: number;
}

/**
 * Generic async Select reused for Group/Device/Sensor/Channel. `allowCustomValue`
 * lets a user type a Grafana template variable (e.g. `$group`) instead of picking
 * a concrete option — Grafana's own variable interpolation takes care of the rest.
 */
export function HierarchySelect({
  inputId,
  'aria-label': ariaLabel,
  placeholder,
  value,
  valueName,
  onChange,
  fetcher,
  extraOptions,
  disabled,
  width = 32,
}: Props) {
  const { options: fetchedOptions, loading, loadOptions } = useResourceOptions(fetcher);

  const options = useMemo(
    () => [...(extraOptions ? extraOptions.map((item) => ({ value: item.id, label: item.name })) : []), ...fetchedOptions],
    [extraOptions, fetchedOptions]
  );

  const selectedValue = useMemo(() => toSelectedValue(value, valueName, options), [value, valueName, options]);

  const handleChange = useCallback(
    (option: SelectableValue<string> | null) => {
      if (!option || option.value === undefined) {
        onChange(undefined, undefined);
        return;
      }
      onChange(option.value, typeof option.label === 'string' ? option.label : option.value);
    },
    [onChange]
  );

  return (
    <AsyncSelect
      inputId={inputId}
      aria-label={ariaLabel}
      isClearable
      allowCustomValue
      isLoading={loading}
      disabled={disabled}
      placeholder={placeholder}
      width={width}
      value={selectedValue}
      defaultOptions={options}
      loadOptions={loadOptions}
      onChange={handleChange}
    />
  );
}
