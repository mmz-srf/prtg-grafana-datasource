import React from 'react';
import { SelectableValue } from '@grafana/data';
import { InlineField, RadioButtonGroup } from '@grafana/ui';

interface Props {
  useRegex: boolean;
  onChange: (useRegex: boolean) => void;
}

const SELECTION_MODE_OPTIONS: Array<SelectableValue<boolean>> = [
  { label: 'Hierarchy', value: false },
  { label: 'Regex match', value: true },
];

/** Toggles between an exact hierarchy pick and a regex multi-sensor/channel match. */
export function SelectionModeSection({ useRegex, onChange }: Props) {
  return (
    <InlineField
      label="Selection mode"
      labelWidth={25}
      tooltip="Pick a single sensor/channel, or match many via a regular expression"
    >
      <RadioButtonGroup<boolean>
        id="query-editor-selection-mode"
        options={SELECTION_MODE_OPTIONS}
        value={useRegex}
        onChange={onChange}
      />
    </InlineField>
  );
}
