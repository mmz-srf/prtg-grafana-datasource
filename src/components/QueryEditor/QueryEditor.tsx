import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../../datasource';
import { PrtgDataSourceOptions, PrtgQuery } from '../../types';
import { HierarchyPicker } from './HierarchyPicker';
import { QueryModeSection } from './QueryModeSection';
import { RegexPicker } from './RegexPicker';
import { SelectionModeSection } from './SelectionModeSection';

type Props = QueryEditorProps<DataSource, PrtgQuery, PrtgDataSourceOptions>;

/**
 * Composes the sections below; holds no state of its own beyond `query`.
 * Switching selection mode preserves both branches' data (cheap, avoids
 * destroying work in progress) — only the active branch is rendered.
 * Every edit calls `onRunQuery()` optimistically since `datasource.filterQuery`
 * is the real gate: an incomplete query is silently skipped, never erroring.
 */
export function QueryEditor({ datasource, query, onChange, onRunQuery }: Props) {
  const useRegex = !!query.useRegex;

  const onSelectionModeChange = (value: boolean) => {
    onChange({ ...query, useRegex: value });
    onRunQuery();
  };

  return (
    <div>
      <QueryModeSection query={query} onChange={onChange} onRunQuery={onRunQuery} />
      <SelectionModeSection useRegex={useRegex} onChange={onSelectionModeChange} />
      {useRegex ? (
        <RegexPicker datasource={datasource} query={query} onChange={onChange} onRunQuery={onRunQuery} />
      ) : (
        <HierarchyPicker datasource={datasource} query={query} onChange={onChange} onRunQuery={onRunQuery} />
      )}
    </div>
  );
}
