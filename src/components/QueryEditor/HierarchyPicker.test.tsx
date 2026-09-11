import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HierarchyPicker } from './HierarchyPicker';
import { DataSource } from '../../datasource';
import { DEFAULT_QUERY, PrtgQuery } from '../../types';

// HierarchySelect wraps @grafana/ui's AsyncSelect (react-select) — its own
// async-loading/typing behavior belongs to that library, not to us. Stubbing
// it out lets these tests focus purely on HierarchyPicker's own cascading
// reset logic, driven deterministically by a single click.
jest.mock('./HierarchySelect', () => ({
  HierarchySelect: ({
    inputId,
    onChange,
    disabled,
  }: {
    inputId?: string;
    onChange: (id?: string, name?: string) => void;
    disabled?: boolean;
  }) => (
    <button data-testid={inputId} disabled={disabled} onClick={() => onChange(`${inputId}-id`, `${inputId}-name`)}>
      {inputId}
    </button>
  ),
}));

function createDatasource(): DataSource {
  return {
    getGroups: jest.fn().mockResolvedValue([]),
    getDevices: jest.fn().mockResolvedValue([]),
    getSensors: jest.fn().mockResolvedValue([]),
    getChannels: jest.fn().mockResolvedValue([]),
  } as unknown as DataSource;
}

function createQuery(overrides: Partial<PrtgQuery> = {}): PrtgQuery {
  return { refId: 'A', ...DEFAULT_QUERY, useRegex: false, ...overrides } as PrtgQuery;
}

describe('HierarchyPicker', () => {
  it('clears every downstream field when the group changes', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    render(
      <HierarchyPicker
        datasource={createDatasource()}
        query={createQuery({
          groupId: 'g1',
          groupName: 'Group 1',
          deviceId: 'd1',
          deviceName: 'Device 1',
          sensorId: 's1',
          sensorName: 'Sensor 1',
          channelId: 'c1',
          channelName: 'Channel 1',
        })}
        onChange={onChange}
        onRunQuery={onRunQuery}
      />
    );

    await userEvent.click(screen.getByTestId('query-editor-group'));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        groupId: 'query-editor-group-id',
        groupName: 'query-editor-group-name',
        deviceId: undefined,
        deviceName: undefined,
        sensorId: undefined,
        sensorName: undefined,
        channelId: undefined,
        channelName: undefined,
      })
    );
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('clears sensor and channel when the device changes, but keeps the group', async () => {
    const onChange = jest.fn();
    render(
      <HierarchyPicker
        datasource={createDatasource()}
        query={createQuery({ groupId: 'g1', groupName: 'Group 1', sensorId: 's1', channelId: 'c1' })}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );

    await userEvent.click(screen.getByTestId('query-editor-device'));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        groupId: 'g1',
        groupName: 'Group 1',
        deviceId: 'query-editor-device-id',
        deviceName: 'query-editor-device-name',
        sensorId: undefined,
        sensorName: undefined,
        channelId: undefined,
        channelName: undefined,
      })
    );
  });

  it('clears only the channel when the sensor changes', async () => {
    const onChange = jest.fn();
    render(
      <HierarchyPicker
        datasource={createDatasource()}
        query={createQuery({ deviceId: 'd1', channelId: 'c1', channelName: 'Channel 1' })}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );

    await userEvent.click(screen.getByTestId('query-editor-sensor'));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        deviceId: 'd1',
        sensorId: 'query-editor-sensor-id',
        sensorName: 'query-editor-sensor-name',
        channelId: undefined,
        channelName: undefined,
      })
    );
  });

  it('updates the channel without touching anything upstream', async () => {
    const onChange = jest.fn();
    render(
      <HierarchyPicker
        datasource={createDatasource()}
        query={createQuery({ groupId: 'g1', deviceId: 'd1', sensorId: 's1' })}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );

    await userEvent.click(screen.getByTestId('query-editor-channel'));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        groupId: 'g1',
        deviceId: 'd1',
        sensorId: 's1',
        channelId: 'query-editor-channel-id',
        channelName: 'query-editor-channel-name',
      })
    );
  });

  it('disables the channel select until a sensor is chosen', () => {
    render(
      <HierarchyPicker
        datasource={createDatasource()}
        query={createQuery()}
        onChange={jest.fn()}
        onRunQuery={jest.fn()}
      />
    );

    expect(screen.getByTestId('query-editor-channel')).toBeDisabled();
  });

  it('enables the channel select once a sensor is chosen', () => {
    render(
      <HierarchyPicker
        datasource={createDatasource()}
        query={createQuery({ sensorId: 's1' })}
        onChange={jest.fn()}
        onRunQuery={jest.fn()}
      />
    );

    expect(screen.getByTestId('query-editor-channel')).toBeEnabled();
  });
});
