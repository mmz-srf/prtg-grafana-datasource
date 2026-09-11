import React, { useCallback } from 'react';
import { InlineField, InlineFieldRow } from '@grafana/ui';
import { DataSource } from '../../datasource';
import { PrtgQuery } from '../../types';
import { HierarchySelect } from './HierarchySelect';
import { HierarchyItem } from './toSelectableValue';

interface Props {
  datasource: DataSource;
  query: PrtgQuery;
  onChange: (query: PrtgQuery) => void;
  onRunQuery: () => void;
}

const ALL_CHANNELS_OPTION: HierarchyItem = { id: '*', name: 'All channels' };
const NO_EXTRA_OPTIONS: HierarchyItem[] = [];

/**
 * Composes the 4 hierarchy selects. Fully controlled off `query.*` — no shadow
 * state — with downstream-only cascading resets: changing a level clears every
 * level below it (Group clears Device/Sensor/Channel, Device clears
 * Sensor/Channel, Sensor clears Channel), but never anything above.
 */
export function HierarchyPicker({ datasource, query, onChange, onRunQuery }: Props) {
  const { groupId, groupName, deviceId, deviceName, sensorId, sensorName, channelId, channelName } = query;

  const fetchGroups = useCallback(() => datasource.getGroups(), [datasource]);
  const fetchDevices = useCallback(() => datasource.getDevices(groupId), [datasource, groupId]);
  const fetchSensors = useCallback(() => datasource.getSensors(deviceId), [datasource, deviceId]);
  const fetchChannels = useCallback(
    () => (sensorId ? datasource.getChannels(sensorId) : Promise.resolve([])),
    [datasource, sensorId]
  );

  const onGroupChange = (id: string | undefined, name: string | undefined) => {
    onChange({
      ...query,
      groupId: id,
      groupName: name,
      deviceId: undefined,
      deviceName: undefined,
      sensorId: undefined,
      sensorName: undefined,
      channelId: undefined,
      channelName: undefined,
    });
    onRunQuery();
  };

  const onDeviceChange = (id: string | undefined, name: string | undefined) => {
    onChange({
      ...query,
      deviceId: id,
      deviceName: name,
      sensorId: undefined,
      sensorName: undefined,
      channelId: undefined,
      channelName: undefined,
    });
    onRunQuery();
  };

  const onSensorChange = (id: string | undefined, name: string | undefined) => {
    onChange({
      ...query,
      sensorId: id,
      sensorName: name,
      channelId: undefined,
      channelName: undefined,
    });
    onRunQuery();
  };

  const onChannelChange = (id: string | undefined, name: string | undefined) => {
    onChange({ ...query, channelId: id, channelName: name });
    onRunQuery();
  };

  return (
    <InlineFieldRow>
      <InlineField label="Group" labelWidth={12} tooltip="PRTG group (or probe) to drill into">
        <HierarchySelect
          inputId="query-editor-group"
          aria-label="Group"
          placeholder="All groups"
          value={groupId}
          valueName={groupName}
          onChange={onGroupChange}
          fetcher={fetchGroups}
        />
      </InlineField>
      <InlineField label="Device" labelWidth={12} tooltip="PRTG device">
        <HierarchySelect
          inputId="query-editor-device"
          aria-label="Device"
          placeholder="All devices"
          value={deviceId}
          valueName={deviceName}
          onChange={onDeviceChange}
          fetcher={fetchDevices}
        />
      </InlineField>
      <InlineField label="Sensor" labelWidth={12} tooltip="PRTG sensor">
        <HierarchySelect
          inputId="query-editor-sensor"
          aria-label="Sensor"
          placeholder="All sensors"
          value={sensorId}
          valueName={sensorName}
          onChange={onSensorChange}
          fetcher={fetchSensors}
        />
      </InlineField>
      <InlineField
        label="Channel"
        labelWidth={12}
        tooltip="PRTG channel — pick 'All channels' to query every channel"
        disabled={!sensorId}
      >
        <HierarchySelect
          inputId="query-editor-channel"
          aria-label="Channel"
          placeholder={sensorId ? 'Select a channel' : 'Select a sensor first'}
          value={channelId}
          valueName={channelName}
          onChange={onChannelChange}
          fetcher={fetchChannels}
          extraOptions={sensorId ? [ALL_CHANNELS_OPTION] : NO_EXTRA_OPTIONS}
        />
      </InlineField>
    </InlineFieldRow>
  );
}
