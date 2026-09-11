import React, { ChangeEvent, useCallback } from 'react';
import { SelectableValue } from '@grafana/data';
import { InlineField, InlineFieldRow, Input, RadioButtonGroup } from '@grafana/ui';
import { DataSource } from '../../datasource';
import { PrtgChannelMatchMode, PrtgQuery } from '../../types';
import { HierarchySelect } from './HierarchySelect';
import { MatchPreview } from './MatchPreview';

interface Props {
  datasource: DataSource;
  query: PrtgQuery;
  onChange: (query: PrtgQuery) => void;
  onRunQuery: () => void;
}

const CHANNEL_MATCH_MODE_OPTIONS: Array<SelectableValue<PrtgChannelMatchMode>> = [
  { label: 'Exact', value: 'exact' },
  { label: 'Regex', value: 'regex' },
];

/**
 * Regex multi-match mode: an optional Group/Device scope narrows the search,
 * a sensor-name regex plus a channel match (exact name, or its own regex)
 * select the channels to fan out to, previewed live by <MatchPreview/>.
 */
export function RegexPicker({ datasource, query, onChange, onRunQuery }: Props) {
  const { groupId, groupName, deviceId, deviceName, sensorPattern, channelPattern } = query;
  const channelMatchMode = query.channelMatchMode ?? 'exact';

  const fetchGroups = useCallback(() => datasource.getGroups(), [datasource]);
  const fetchDevices = useCallback(() => datasource.getDevices(groupId), [datasource, groupId]);

  const onGroupChange = (id: string | undefined, name: string | undefined) => {
    onChange({ ...query, groupId: id, groupName: name, deviceId: undefined, deviceName: undefined });
    onRunQuery();
  };

  const onDeviceChange = (id: string | undefined, name: string | undefined) => {
    onChange({ ...query, deviceId: id, deviceName: name });
    onRunQuery();
  };

  const onSensorPatternChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, sensorPattern: event.target.value });
    onRunQuery();
  };

  const onChannelMatchModeChange = (value: PrtgChannelMatchMode) => {
    onChange({ ...query, channelMatchMode: value });
    onRunQuery();
  };

  const onChannelPatternChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, channelPattern: event.target.value });
    onRunQuery();
  };

  return (
    <>
      <InlineFieldRow>
        <InlineField label="Group scope" labelWidth={25} tooltip="Optionally limit the search to a group">
          <HierarchySelect
            inputId="query-editor-regex-group"
            aria-label="Group scope"
            placeholder="All groups"
            value={groupId}
            valueName={groupName}
            onChange={onGroupChange}
            fetcher={fetchGroups}
          />
        </InlineField>
        <InlineField label="Device scope" labelWidth={25} tooltip="Optionally limit the search to a device">
          <HierarchySelect
            inputId="query-editor-regex-device"
            aria-label="Device scope"
            placeholder="All devices"
            value={deviceId}
            valueName={deviceName}
            onChange={onDeviceChange}
            fetcher={fetchDevices}
          />
        </InlineField>
      </InlineFieldRow>
      <InlineField
        label="Sensor pattern"
        labelWidth={25}
        tooltip="Regular expression matched against sensor names"
        grow
      >
        <Input
          id="query-editor-sensor-pattern"
          onChange={onSensorPatternChange}
          value={sensorPattern ?? ''}
          placeholder="e.g. CPU Load.*"
        />
      </InlineField>
      <InlineFieldRow>
        <InlineField label="Channel match" labelWidth={25} tooltip="Match the channel name exactly, or via regex">
          <RadioButtonGroup<PrtgChannelMatchMode>
            id="query-editor-channel-match-mode"
            options={CHANNEL_MATCH_MODE_OPTIONS}
            value={channelMatchMode}
            onChange={onChannelMatchModeChange}
          />
        </InlineField>
        <InlineField
          label="Channel pattern"
          labelWidth={25}
          tooltip={
            channelMatchMode === 'regex' ? 'Regular expression matched against channel names' : 'Exact channel name'
          }
          grow
        >
          <Input
            id="query-editor-channel-pattern"
            onChange={onChannelPatternChange}
            value={channelPattern ?? ''}
            placeholder={channelMatchMode === 'regex' ? 'e.g. Downtime.*' : 'e.g. Downtime'}
          />
        </InlineField>
      </InlineFieldRow>
      <MatchPreview
        datasource={datasource}
        groupId={groupId}
        deviceId={deviceId}
        sensorPattern={sensorPattern}
        channelMatchMode={channelMatchMode}
        channelPattern={channelPattern}
      />
    </>
  );
}
