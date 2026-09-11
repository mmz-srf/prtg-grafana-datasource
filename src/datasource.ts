import { CoreApp, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';

import {
  DEFAULT_QUERY,
  PrtgChannel,
  PrtgChannelMatchMode,
  PrtgDataSourceOptions,
  PrtgDevice,
  PrtgGroup,
  PrtgQuery,
  PrtgSensor,
  PrtgSensorSearchResult,
} from './types';

export interface SearchSensorsParams {
  pattern: string;
  groupId?: string;
  deviceId?: string;
  channelMatchMode?: PrtgChannelMatchMode;
  channelPattern?: string;
}

/** Keys used for the "no scope" case of a Promise cache, so `undefined` and `''` share a slot. */
const UNSCOPED_KEY = '';

export class DataSource extends DataSourceWithBackend<PrtgQuery, PrtgDataSourceOptions> {
  // Instance-scoped promise caches: Grafana reuses DataSourceApi instances, so
  // these dedupe concurrent + repeat calls for the lifetime of the instance.
  // Rejections are removed so a transient failure isn't sticky.
  private groupsCache = new Map<string, Promise<PrtgGroup[]>>();
  private devicesCache = new Map<string, Promise<PrtgDevice[]>>();
  private sensorsCache = new Map<string, Promise<PrtgSensor[]>>();
  private channelsCache = new Map<string, Promise<PrtgChannel[]>>();

  constructor(instanceSettings: DataSourceInstanceSettings<PrtgDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<PrtgQuery> {
    return DEFAULT_QUERY;
  }

  getGroups(): Promise<PrtgGroup[]> {
    return this.cached(this.groupsCache, UNSCOPED_KEY, () => this.getResource<PrtgGroup[]>('groups'));
  }

  getDevices(groupId?: string): Promise<PrtgDevice[]> {
    const key = groupId || UNSCOPED_KEY;
    return this.cached(this.devicesCache, key, () =>
      this.getResource<PrtgDevice[]>('devices', groupId ? { groupId } : undefined)
    );
  }

  getSensors(deviceId?: string): Promise<PrtgSensor[]> {
    const key = deviceId || UNSCOPED_KEY;
    return this.cached(this.sensorsCache, key, () =>
      this.getResource<PrtgSensor[]>('sensors', deviceId ? { deviceId } : undefined)
    );
  }

  getChannels(sensorId: string): Promise<PrtgChannel[]> {
    return this.cached(this.channelsCache, sensorId, () => this.getResource<PrtgChannel[]>('channels', { sensorId }));
  }

  /** Uncached — drives the live-typing match preview, so results must always be fresh. */
  searchSensors(params: SearchSensorsParams): Promise<PrtgSensorSearchResult> {
    return this.getResource<PrtgSensorSearchResult>('sensors/search', params);
  }

  /** Manual refresh affordance: drop every cached hierarchy list. */
  clearMetadataCache(): void {
    this.groupsCache.clear();
    this.devicesCache.clear();
    this.sensorsCache.clear();
    this.channelsCache.clear();
  }

  private cached<T>(cache: Map<string, Promise<T>>, key: string, fetch: () => Promise<T>): Promise<T> {
    const existing = cache.get(key);
    if (existing) {
      return existing;
    }
    const promise = fetch().catch((err) => {
      cache.delete(key);
      throw err;
    });
    cache.set(key, promise);
    return promise;
  }

  applyTemplateVariables(query: PrtgQuery, scopedVars: ScopedVars): PrtgQuery {
    const templateSrv = getTemplateSrv();
    const replace = (value?: string): string | undefined =>
      value === undefined ? value : templateSrv.replace(value, scopedVars);

    return {
      ...query,
      groupId: replace(query.groupId),
      groupName: replace(query.groupName),
      deviceId: replace(query.deviceId),
      deviceName: replace(query.deviceName),
      sensorId: replace(query.sensorId),
      sensorName: replace(query.sensorName),
      channelId: replace(query.channelId),
      channelName: replace(query.channelName),
      sensorPattern: replace(query.sensorPattern),
      channelPattern: replace(query.channelPattern),
    };
  }

  filterQuery(query: PrtgQuery): boolean {
    if (query.useRegex) {
      const sensorPattern = query.sensorPattern?.trim();
      const channelPattern = query.channelPattern?.trim();
      if (!sensorPattern || !channelPattern) {
        return false;
      }
      if (!this.isValidPattern(sensorPattern)) {
        return false;
      }
      if (query.channelMatchMode === 'regex' && !this.isValidPattern(channelPattern)) {
        return false;
      }
      return true;
    }

    return !!query.sensorId?.trim() && !!query.channelId?.trim();
  }

  /** A pattern is valid if it contains a template variable (resolved later) or compiles as a RegExp. */
  private isValidPattern(pattern: string): boolean {
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
}
