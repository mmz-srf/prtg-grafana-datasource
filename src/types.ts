import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

/**
 * Query modes: a "current value" snapshot, or a "historic" time series
 * (PRTG API v2 exposes only fixed windows for the latter, see the backend).
 */
export type PrtgQueryType = 'current' | 'timeseries';

/** How the channel pattern is interpreted in regex ("useRegex") mode. */
export type PrtgChannelMatchMode = 'exact' | 'regex';

/**
 * Flat query model shared with the Go backend (`pkg/plugin/query.go`) — see the
 * plan's unified contract. Both the hierarchy-pick fields and the regex-match
 * fields coexist; whichever set is unused for the current `useRegex` value is
 * simply ignored by the backend.
 */
export interface PrtgQuery extends DataQuery {
  queryType: PrtgQueryType;
  /** false/undefined = exact hierarchy pick, true = regex multi-match. */
  useRegex?: boolean;

  // Exact selection (useRegex=false) OR scope filter (useRegex=true).
  // The *Name fields are display-only: they let a picker show a saved
  // selection's label before its async option list has loaded, and are
  // ignored by the backend.
  groupId?: string;
  groupName?: string;
  deviceId?: string;
  deviceName?: string;
  sensorId?: string;
  sensorName?: string;
  /** '*' or empty = all channels of the sensor. */
  channelId?: string;
  channelName?: string;

  // Regex mode only (useRegex=true) — groupId/deviceId above become an
  // optional scope filter instead of an exact selection.
  /** Go regexp matched against sensor names. */
  sensorPattern?: string;
  /** default 'exact'. */
  channelMatchMode?: PrtgChannelMatchMode;
  /** Exact channel name, or regex per channelMatchMode. */
  channelPattern?: string;
}

export const DEFAULT_QUERY: Partial<PrtgQuery> = {
  queryType: 'current',
  channelMatchMode: 'exact',
};

/** How the datasource authenticates against the PRTG API v2. */
export type PrtgAuthMode = 'apiKey' | 'credentials';

/**
 * Non-secret options configured for each DataSource instance (`jsonData`).
 */
export interface PrtgDataSourceOptions extends DataSourceJsonData {
  serverUrl?: string;
  authMode?: PrtgAuthMode;
  username?: string;
  tlsSkipVerify?: boolean;
}

/**
 * Secret values that are configured for each DataSource instance
 * (`secureJsonData`) — never sent back to the frontend once set.
 */
export interface PrtgSecureJsonData {
  apiKey?: string;
  password?: string;
}

/** `GET groups` — top-level groups (probes are folded into this tier too). */
export interface PrtgGroup {
  id: string;
  name: string;
}

/** `GET devices?groupId=` */
export interface PrtgDevice {
  id: string;
  name: string;
  groupId?: string;
}

/** `GET sensors?deviceId=` */
export interface PrtgSensor {
  id: string;
  name: string;
  deviceId?: string;
}

/** `GET channels?sensorId=` */
export interface PrtgChannel {
  id: string;
  name: string;
  sensorId?: string;
  /** The channel's display unit (e.g. "kbit/s"), if PRTG reports one. */
  unit?: string;
}

/** A single channel matched within a sensor by `sensors/search`. */
export interface PrtgSensorSearchMatchedChannel {
  id: string;
  name: string;
}

/** A single sensor matched by `sensors/search`, with its matched channels. */
export interface PrtgSensorSearchMatch {
  id: string;
  name: string;
  deviceId: string;
  deviceName: string;
  matchedChannels: PrtgSensorSearchMatchedChannel[];
}

/** `GET sensors/search?pattern=&groupId=&deviceId=&channelMatchMode=&channelPattern=` */
export interface PrtgSensorSearchResult {
  sensors: PrtgSensorSearchMatch[];
  totalSensors: number;
  totalChannels: number;
  truncated: boolean;
}
