import React from 'react';
import { SelectableValue } from '@grafana/data';
import { Alert, InlineField, RadioButtonGroup } from '@grafana/ui';
import { PrtgQuery, PrtgQueryType } from '../../types';

interface Props {
  query: PrtgQuery;
  onChange: (query: PrtgQuery) => void;
  onRunQuery: () => void;
}

const QUERY_TYPE_OPTIONS: Array<SelectableValue<PrtgQueryType>> = [
  { label: 'Current value', value: 'current' },
  { label: 'Historic time series', value: 'timeseries' },
];

/**
 * Current/historic toggle. When historic is selected, shows a persistent
 * info Alert explaining PRTG's fixed windows — the backend also attaches a
 * frame Notice, but that's easy to miss in the panel UI, so this is repeated
 * here in the editor.
 */
export function QueryModeSection({ query, onChange, onRunQuery }: Props) {
  const queryType = query.queryType ?? 'current';

  const onQueryTypeChange = (value: PrtgQueryType) => {
    onChange({ ...query, queryType: value });
    onRunQuery();
  };

  return (
    <>
      <InlineField label="Query type" labelWidth={25} tooltip="Current live value, or a historic time series">
        <RadioButtonGroup<PrtgQueryType>
          id="query-editor-query-type"
          options={QUERY_TYPE_OPTIONS}
          value={queryType}
          onChange={onQueryTypeChange}
        />
      </InlineField>
      {queryType === 'timeseries' && (
        <Alert severity="info" title="PRTG API v2 uses fixed time windows">
          Historic data is only available in fixed windows (last 4 hours, 2 days, 60 days, or 365 days) — PRTG, not
          Grafana, dictates the resolution. The backend picks the smallest window covering the dashboard&apos;s time
          range and trims it to fit; a range wider than 365 days is clamped to the longest window.
        </Alert>
      )}
    </>
  );
}
