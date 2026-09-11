import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { MatchPreview } from './MatchPreview';
import { DataSource } from '../../datasource';
import { PrtgSensorSearchResult } from '../../types';

// MatchPreview checks for template variables via getTemplateSrv(), which isn't
// bootstrapped outside a running Grafana instance.
jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  getTemplateSrv: () => ({
    replace: (value?: string) => value ?? '',
    containsTemplate: () => false,
  }),
}));

function createDatasource(searchSensors: jest.Mock): DataSource {
  return { searchSensors } as unknown as DataSource;
}

function searchResult(overrides: Partial<PrtgSensorSearchResult>): PrtgSensorSearchResult {
  return { sensors: [], totalSensors: 0, totalChannels: 0, truncated: false, ...overrides };
}

describe('MatchPreview', () => {
  it('renders nothing and never searches when the patterns are empty', () => {
    const searchSensors = jest.fn();
    render(<MatchPreview datasource={createDatasource(searchSensors)} sensorPattern="" channelPattern="" />);

    expect(searchSensors).not.toHaveBeenCalled();
    expect(screen.queryByText(/matched/i)).not.toBeInTheDocument();
  });

  it('shows an inline error for an invalid sensor pattern and never calls searchSensors', async () => {
    const searchSensors = jest.fn();
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="("
        channelPattern="anything"
        channelMatchMode="exact"
      />
    );

    await screen.findByText('Sensor pattern is not a valid regular expression.');
    expect(searchSensors).not.toHaveBeenCalled();
  });

  it('shows an inline error for an invalid channel regex and never calls searchSensors', async () => {
    const searchSensors = jest.fn();
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="CPU.*"
        channelPattern="("
        channelMatchMode="regex"
      />
    );

    await screen.findByText('Channel pattern is not a valid regular expression.');
    expect(searchSensors).not.toHaveBeenCalled();
  });

  it('does not flag an exact (non-regex) channel pattern containing regex metacharacters', async () => {
    const searchSensors = jest.fn().mockResolvedValue(searchResult({ totalSensors: 1, totalChannels: 1 }));
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="CPU.*"
        channelPattern="("
        channelMatchMode="exact"
      />
    );

    await waitFor(() => expect(searchSensors).toHaveBeenCalled());
    expect(screen.queryByText(/not a valid regular expression/i)).not.toBeInTheDocument();
  });

  it('shows a 0-match warning', async () => {
    const searchSensors = jest.fn().mockResolvedValue(searchResult({}));
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="CPU.*"
        channelPattern="Downtime"
        channelMatchMode="exact"
      />
    );

    await screen.findByText('No sensors or channels matched the current pattern.');
    expect(searchSensors).toHaveBeenCalledWith({
      pattern: 'CPU.*',
      groupId: undefined,
      deviceId: undefined,
      channelMatchMode: 'exact',
      channelPattern: 'Downtime',
    });
  });

  it('shows the match count on success', async () => {
    const searchSensors = jest.fn().mockResolvedValue(searchResult({ totalSensors: 3, totalChannels: 5 }));
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="CPU.*"
        channelPattern="Downtime"
        channelMatchMode="exact"
      />
    );

    await screen.findByText('3 sensor(s), 5 channel(s) matched.');
  });

  it('flags a truncated result', async () => {
    const searchSensors = jest
      .fn()
      .mockResolvedValue(searchResult({ totalSensors: 100, totalChannels: 200, truncated: true }));
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="CPU.*"
        channelPattern="Downtime"
        channelMatchMode="exact"
      />
    );

    await screen.findByText(/truncated/i);
  });

  it('surfaces a network error', async () => {
    const searchSensors = jest.fn().mockRejectedValue(new Error('boom'));
    render(
      <MatchPreview
        datasource={createDatasource(searchSensors)}
        sensorPattern="CPU.*"
        channelPattern="Downtime"
        channelMatchMode="exact"
      />
    );

    await screen.findByText('boom');
  });

  it('debounces rapid pattern changes into a single search call', async () => {
    const searchSensors = jest.fn().mockResolvedValue(searchResult({ totalSensors: 1, totalChannels: 1 }));
    const datasource = createDatasource(searchSensors);
    // Mount empty (as a real editor does before the user types anything) so
    // the first non-empty value below is itself a debounced *change*, not an
    // immediately-seeded initial value.
    const { rerender } = render(
      <MatchPreview datasource={datasource} sensorPattern="" channelPattern="" channelMatchMode="exact" />
    );
    rerender(
      <MatchPreview datasource={datasource} sensorPattern="C" channelPattern="Downtime" channelMatchMode="exact" />
    );
    rerender(
      <MatchPreview datasource={datasource} sensorPattern="CP" channelPattern="Downtime" channelMatchMode="exact" />
    );
    rerender(
      <MatchPreview
        datasource={datasource}
        sensorPattern="CPU.*"
        channelPattern="Downtime"
        channelMatchMode="exact"
      />
    );

    await waitFor(() => expect(searchSensors).toHaveBeenCalledTimes(1));
    expect(searchSensors).toHaveBeenCalledWith(expect.objectContaining({ pattern: 'CPU.*' }));
  });
});
