import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RegexPicker } from './RegexPicker';
import { DataSource } from '../../datasource';
import { DEFAULT_QUERY, PrtgQuery } from '../../types';

jest.mock('./HierarchySelect', () => ({
  HierarchySelect: ({
    inputId,
    onChange,
  }: {
    inputId?: string;
    onChange: (id?: string, name?: string) => void;
  }) => (
    <button data-testid={inputId} onClick={() => onChange('scope-1', 'Scope One')}>
      {inputId}
    </button>
  ),
}));

jest.mock('./MatchPreview', () => ({
  MatchPreview: (props: Record<string, unknown>) => <div data-testid="match-preview">{JSON.stringify(props)}</div>,
}));

function createDatasource(): DataSource {
  return {
    getGroups: jest.fn().mockResolvedValue([]),
    getDevices: jest.fn().mockResolvedValue([]),
  } as unknown as DataSource;
}

function createQuery(overrides: Partial<PrtgQuery> = {}): PrtgQuery {
  return { refId: 'A', ...DEFAULT_QUERY, useRegex: true, ...overrides } as PrtgQuery;
}

describe('RegexPicker', () => {
  it('clears the device scope when the group scope changes', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    render(
      <RegexPicker
        datasource={createDatasource()}
        query={createQuery({ groupId: 'g1', groupName: 'Group 1', deviceId: 'd1', deviceName: 'Device 1' })}
        onChange={onChange}
        onRunQuery={onRunQuery}
      />
    );

    await userEvent.click(screen.getByTestId('query-editor-regex-group'));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        groupId: 'scope-1',
        groupName: 'Scope One',
        deviceId: undefined,
        deviceName: undefined,
      })
    );
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('updates the device scope without touching the group scope', async () => {
    const onChange = jest.fn();
    render(
      <RegexPicker
        datasource={createDatasource()}
        query={createQuery({ groupId: 'g1', groupName: 'Group 1' })}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );

    await userEvent.click(screen.getByTestId('query-editor-regex-device'));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ groupId: 'g1', groupName: 'Group 1', deviceId: 'scope-1', deviceName: 'Scope One' })
    );
  });

  it('updates the sensor pattern and calls onRunQuery', () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    render(
      <RegexPicker
        datasource={createDatasource()}
        query={createQuery()}
        onChange={onChange}
        onRunQuery={onRunQuery}
      />
    );

    fireEvent.change(screen.getByPlaceholderText('e.g. CPU Load.*'), { target: { value: 'CPU Load.*' } });

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ sensorPattern: 'CPU Load.*' }));
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('updates the channel pattern and calls onRunQuery', () => {
    const onChange = jest.fn();
    render(
      <RegexPicker datasource={createDatasource()} query={createQuery()} onChange={onChange} onRunQuery={jest.fn()} />
    );

    fireEvent.change(screen.getByPlaceholderText('e.g. Downtime'), { target: { value: 'Downtime' } });

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ channelPattern: 'Downtime' }));
  });

  it('switches the channel match mode to regex and updates the pattern placeholder', async () => {
    const onChange = jest.fn();
    const { rerender } = render(
      <RegexPicker datasource={createDatasource()} query={createQuery()} onChange={onChange} onRunQuery={jest.fn()} />
    );

    await userEvent.click(screen.getByRole('radio', { name: 'Regex' }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ channelMatchMode: 'regex' }));

    rerender(
      <RegexPicker
        datasource={createDatasource()}
        query={createQuery({ channelMatchMode: 'regex' })}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );

    expect(screen.getByPlaceholderText('e.g. Downtime.*')).toBeInTheDocument();
  });

  it('passes the current patterns through to the match preview', () => {
    render(
      <RegexPicker
        datasource={createDatasource()}
        query={createQuery({ sensorPattern: 'CPU.*', channelPattern: 'Downtime' })}
        onChange={jest.fn()}
        onRunQuery={jest.fn()}
      />
    );

    const preview = screen.getByTestId('match-preview');
    expect(preview).toHaveTextContent('CPU.*');
    expect(preview).toHaveTextContent('Downtime');
  });
});
