import React, { useEffect, useMemo, useState } from 'react';
import { Alert, type AlertVariant } from '@grafana/ui';
import { getTemplateSrv } from '@grafana/runtime';
import { DataSource } from '../../datasource';
import { PrtgChannelMatchMode } from '../../types';
import { useDebouncedValue } from './useDebouncedValue';

interface Props {
  datasource: DataSource;
  groupId?: string;
  deviceId?: string;
  sensorPattern?: string;
  channelMatchMode?: PrtgChannelMatchMode;
  channelPattern?: string;
}

type PreviewStatus = 'idle' | 'loading' | 'invalid' | 'empty' | 'ok' | 'error';

interface PreviewState {
  status: PreviewStatus;
  message?: string;
}

const IDLE_STATE: PreviewState = { status: 'idle' };

function isValidPattern(pattern: string): boolean {
  if (getTemplateSrv().containsTemplate(pattern)) {
    return true;
  }
  try {
    new RegExp(pattern);
    return true;
  } catch {
    return false;
  }
}

const SEVERITY_BY_STATUS: Record<PreviewStatus, AlertVariant> = {
  idle: 'info',
  loading: 'info',
  ok: 'info',
  empty: 'warning',
  invalid: 'error',
  error: 'error',
};

/**
 * ~400ms debounced live preview of a regex sensor/channel match. Validates
 * both patterns client-side before ever calling `datasource.searchSensors`,
 * so an invalid regex never triggers a network call.
 */
export function MatchPreview({ datasource, groupId, deviceId, sensorPattern, channelMatchMode, channelPattern }: Props) {
  const debouncedSensorPattern = useDebouncedValue(sensorPattern?.trim() ?? '', 400);
  const debouncedChannelPattern = useDebouncedValue(channelPattern?.trim() ?? '', 400);
  const resolvedChannelMatchMode = channelMatchMode ?? 'exact';
  const [state, setState] = useState<PreviewState>(IDLE_STATE);

  const sensorPatternValid = useMemo(
    () => !debouncedSensorPattern || isValidPattern(debouncedSensorPattern),
    [debouncedSensorPattern]
  );
  const channelPatternValid = useMemo(
    () =>
      resolvedChannelMatchMode !== 'regex' || !debouncedChannelPattern || isValidPattern(debouncedChannelPattern),
    [resolvedChannelMatchMode, debouncedChannelPattern]
  );

  useEffect(() => {
    // Each branch below synchronously syncs `state` to the (already debounced,
    // already validated) pattern inputs before kicking off the async search —
    // the standard data-fetching-in-an-effect pattern
    // (https://react.dev/learn/synchronizing-with-effects#fetching-data).
    if (!debouncedSensorPattern || !debouncedChannelPattern) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setState(IDLE_STATE);
      return;
    }
    if (!sensorPatternValid) {
      setState({ status: 'invalid', message: 'Sensor pattern is not a valid regular expression.' });
      return;
    }
    if (!channelPatternValid) {
      setState({ status: 'invalid', message: 'Channel pattern is not a valid regular expression.' });
      return;
    }

    let cancelled = false;
    setState({ status: 'loading', message: 'Searching for matches…' });

    datasource
      .searchSensors({
        pattern: debouncedSensorPattern,
        groupId,
        deviceId,
        channelMatchMode: resolvedChannelMatchMode,
        channelPattern: debouncedChannelPattern,
      })
      .then((result) => {
        if (cancelled) {
          return;
        }
        if (result.totalSensors === 0 || result.totalChannels === 0) {
          setState({ status: 'empty', message: 'No sensors or channels matched the current pattern.' });
          return;
        }
        const truncatedNote = result.truncated ? ' (truncated — refine your pattern to narrow the match)' : '';
        setState({
          status: 'ok',
          message: `${result.totalSensors} sensor(s), ${result.totalChannels} channel(s) matched${truncatedNote}.`,
        });
      })
      .catch((err: unknown) => {
        if (cancelled) {
          return;
        }
        setState({ status: 'error', message: err instanceof Error ? err.message : 'Failed to search sensors.' });
      });

    return () => {
      cancelled = true;
    };
  }, [
    datasource,
    debouncedSensorPattern,
    debouncedChannelPattern,
    sensorPatternValid,
    channelPatternValid,
    groupId,
    deviceId,
    resolvedChannelMatchMode,
  ]);

  if (!state.message) {
    return null;
  }

  return (
    <Alert severity={SEVERITY_BY_STATUS[state.status]} title="Match preview">
      {state.message}
    </Alert>
  );
}
