import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryEditor } from './QueryEditor';
import { DataSource } from '../../datasource';
import { DEFAULT_QUERY, PrtgQuery } from '../../types';

function createDatasource(): DataSource {
  return {
    getGroups: jest.fn().mockResolvedValue([]),
    getDevices: jest.fn().mockResolvedValue([]),
    getSensors: jest.fn().mockResolvedValue([]),
    getChannels: jest.fn().mockResolvedValue([]),
    searchSensors: jest.fn().mockResolvedValue({ sensors: [], totalSensors: 0, totalChannels: 0, truncated: false }),
  } as unknown as DataSource;
}

function createQuery(overrides: Partial<PrtgQuery> = {}): PrtgQuery {
  return { refId: 'A', ...DEFAULT_QUERY, ...overrides } as PrtgQuery;
}

describe('QueryEditor', () => {
  it('renders the query type and selection mode controls', () => {
    render(
      <QueryEditor datasource={createDatasource()} query={createQuery()} onChange={jest.fn()} onRunQuery={jest.fn()} />
    );

    expect(screen.getByRole('radio', { name: 'Current value' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Historic time series' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Hierarchy' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Regex match' })).toBeInTheDocument();
  });

  it('shows the hierarchy picker by default and not the regex picker', () => {
    render(
      <QueryEditor datasource={createDatasource()} query={createQuery()} onChange={jest.fn()} onRunQuery={jest.fn()} />
    );

    expect(screen.getByLabelText('Group')).toBeInTheDocument();
    expect(screen.queryByPlaceholderText('e.g. CPU Load.*')).not.toBeInTheDocument();
  });

  it('switches to the regex picker and calls onRunQuery', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    const { rerender } = render(
      <QueryEditor
        datasource={createDatasource()}
        query={createQuery()}
        onChange={onChange}
        onRunQuery={onRunQuery}
      />
    );

    await userEvent.click(screen.getByRole('radio', { name: 'Regex match' }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ useRegex: true }));
    expect(onRunQuery).toHaveBeenCalledTimes(1);

    rerender(
      <QueryEditor
        datasource={createDatasource()}
        query={createQuery({ useRegex: true })}
        onChange={onChange}
        onRunQuery={onRunQuery}
      />
    );

    expect(screen.getByPlaceholderText('e.g. CPU Load.*')).toBeInTheDocument();
    expect(screen.queryByLabelText('Group')).not.toBeInTheDocument();
  });

  it('shows the fixed-window alert only when historic time series is selected', async () => {
    const onChange = jest.fn();
    const { rerender } = render(
      <QueryEditor datasource={createDatasource()} query={createQuery()} onChange={onChange} onRunQuery={jest.fn()} />
    );

    expect(screen.queryByText(/fixed time windows/i)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('radio', { name: 'Historic time series' }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ queryType: 'timeseries' }));

    rerender(
      <QueryEditor
        datasource={createDatasource()}
        query={createQuery({ queryType: 'timeseries' })}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );

    expect(screen.getByText(/fixed time windows/i)).toBeInTheDocument();
  });
});
