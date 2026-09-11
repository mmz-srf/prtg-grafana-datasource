import { DataSourceInstanceSettings } from '@grafana/data';
import { DataSource } from './datasource';
import { PrtgDataSourceOptions, PrtgQuery } from './types';

// filterQuery/applyTemplateVariables consult getTemplateSrv(), which isn't
// bootstrapped outside a running Grafana instance.
let containsTemplateResult = false;
jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  getTemplateSrv: () => ({
    replace: (value?: string) => (value === undefined ? value : `replaced(${value})`),
    containsTemplate: () => containsTemplateResult,
  }),
}));

function createDatasource(): DataSource {
  const instanceSettings = {
    id: 1,
    uid: 'test-uid',
    type: 'srgssr-prtg-datasource',
    name: 'PRTG',
    jsonData: {},
  } as unknown as DataSourceInstanceSettings<PrtgDataSourceOptions>;
  const datasource = new DataSource(instanceSettings);
  // getResource is provided by DataSourceWithBackend and talks to the real
  // Grafana backend_srv — stub it out so the cache/gating logic under test
  // never needs a running Grafana instance.
  jest.spyOn(datasource, 'getResource').mockImplementation(jest.fn());
  return datasource;
}

function createQuery(overrides: Partial<PrtgQuery> = {}): PrtgQuery {
  return { refId: 'A', queryType: 'current', ...overrides };
}

beforeEach(() => {
  containsTemplateResult = false;
});

describe('DataSource', () => {
  describe('metadata caching', () => {
    it('caches getGroups so a repeat call does not hit getResource again', async () => {
      const datasource = createDatasource();
      (datasource.getResource as jest.Mock).mockResolvedValue([{ id: 'g1', name: 'Group 1' }]);

      await datasource.getGroups();
      await datasource.getGroups();

      expect(datasource.getResource).toHaveBeenCalledTimes(1);
      expect(datasource.getResource).toHaveBeenCalledWith('groups');
    });

    it('scopes the getDevices cache by groupId', async () => {
      const datasource = createDatasource();
      (datasource.getResource as jest.Mock).mockResolvedValue([]);

      await datasource.getDevices('g1');
      await datasource.getDevices('g2');
      await datasource.getDevices('g1');

      expect(datasource.getResource).toHaveBeenCalledTimes(2);
      expect(datasource.getResource).toHaveBeenCalledWith('devices', { groupId: 'g1' });
      expect(datasource.getResource).toHaveBeenCalledWith('devices', { groupId: 'g2' });
    });

    it('dedupes concurrent in-flight calls for the same key', async () => {
      const datasource = createDatasource();
      let resolveFetch: (value: unknown) => void = () => {};
      (datasource.getResource as jest.Mock).mockImplementation(
        () => new Promise((resolve) => (resolveFetch = resolve))
      );

      const first = datasource.getSensors('d1');
      const second = datasource.getSensors('d1');
      resolveFetch([{ id: 's1', name: 'Sensor 1' }]);

      await expect(first).resolves.toEqual([{ id: 's1', name: 'Sensor 1' }]);
      await expect(second).resolves.toEqual([{ id: 's1', name: 'Sensor 1' }]);
      expect(datasource.getResource).toHaveBeenCalledTimes(1);
    });

    it('does not cache a rejection, so a retry hits getResource again', async () => {
      const datasource = createDatasource();
      (datasource.getResource as jest.Mock).mockRejectedValueOnce(new Error('boom'));
      (datasource.getResource as jest.Mock).mockResolvedValueOnce([{ id: 'c1', name: 'Channel 1' }]);

      await expect(datasource.getChannels('s1')).rejects.toThrow('boom');
      await expect(datasource.getChannels('s1')).resolves.toEqual([{ id: 'c1', name: 'Channel 1' }]);
      expect(datasource.getResource).toHaveBeenCalledTimes(2);
    });

    it('clearMetadataCache forces every list to be refetched', async () => {
      const datasource = createDatasource();
      (datasource.getResource as jest.Mock).mockResolvedValue([]);

      await datasource.getGroups();
      datasource.clearMetadataCache();
      await datasource.getGroups();

      expect(datasource.getResource).toHaveBeenCalledTimes(2);
    });

    it('never caches searchSensors', async () => {
      const datasource = createDatasource();
      (datasource.getResource as jest.Mock).mockResolvedValue({
        sensors: [],
        totalSensors: 0,
        totalChannels: 0,
        truncated: false,
      });

      await datasource.searchSensors({ pattern: 'CPU.*' });
      await datasource.searchSensors({ pattern: 'CPU.*' });

      expect(datasource.getResource).toHaveBeenCalledTimes(2);
      expect(datasource.getResource).toHaveBeenCalledWith('sensors/search', { pattern: 'CPU.*' });
    });
  });

  describe('filterQuery', () => {
    it('rejects hierarchy mode without a sensorId+channelId', () => {
      const datasource = createDatasource();
      expect(datasource.filterQuery(createQuery({ useRegex: false }))).toBe(false);
      expect(datasource.filterQuery(createQuery({ useRegex: false, sensorId: 's1' }))).toBe(false);
    });

    it('accepts hierarchy mode once sensorId+channelId are set (including the "*" all-channels marker)', () => {
      const datasource = createDatasource();
      expect(datasource.filterQuery(createQuery({ useRegex: false, sensorId: 's1', channelId: 'c1' }))).toBe(true);
      expect(datasource.filterQuery(createQuery({ useRegex: false, sensorId: 's1', channelId: '*' }))).toBe(true);
    });

    it('rejects regex mode with a missing pattern', () => {
      const datasource = createDatasource();
      expect(datasource.filterQuery(createQuery({ useRegex: true, sensorPattern: 'CPU.*' }))).toBe(false);
      expect(datasource.filterQuery(createQuery({ useRegex: true, channelPattern: 'Down' }))).toBe(false);
    });

    it('rejects regex mode with an invalid sensor or channel regex', () => {
      const datasource = createDatasource();
      expect(
        datasource.filterQuery(createQuery({ useRegex: true, sensorPattern: '(', channelPattern: 'Down' }))
      ).toBe(false);
      expect(
        datasource.filterQuery(
          createQuery({ useRegex: true, sensorPattern: 'CPU.*', channelMatchMode: 'regex', channelPattern: '(' })
        )
      ).toBe(false);
    });

    it('does not validate the channel pattern as regex when channelMatchMode is exact', () => {
      const datasource = createDatasource();
      expect(
        datasource.filterQuery(
          createQuery({ useRegex: true, sensorPattern: 'CPU.*', channelMatchMode: 'exact', channelPattern: '(' })
        )
      ).toBe(true);
    });

    it('accepts an otherwise-invalid regex when it contains a template variable', () => {
      containsTemplateResult = true;
      const datasource = createDatasource();
      expect(
        datasource.filterQuery(createQuery({ useRegex: true, sensorPattern: '$myVar', channelPattern: 'Down' }))
      ).toBe(true);
    });
  });

  describe('applyTemplateVariables', () => {
    it('interpolates every PRTG string field', () => {
      const datasource = createDatasource();
      const query = createQuery({
        groupId: 'g1',
        groupName: 'Group 1',
        deviceId: 'd1',
        deviceName: 'Device 1',
        sensorId: 's1',
        sensorName: 'Sensor 1',
        channelId: 'c1',
        channelName: 'Channel 1',
        sensorPattern: 'CPU.*',
        channelPattern: 'Down',
      });

      const result = datasource.applyTemplateVariables(query, {});

      expect(result).toMatchObject({
        groupId: 'replaced(g1)',
        groupName: 'replaced(Group 1)',
        deviceId: 'replaced(d1)',
        deviceName: 'replaced(Device 1)',
        sensorId: 'replaced(s1)',
        sensorName: 'replaced(Sensor 1)',
        channelId: 'replaced(c1)',
        channelName: 'replaced(Channel 1)',
        sensorPattern: 'replaced(CPU.*)',
        channelPattern: 'replaced(Down)',
      });
    });

    it('leaves unset fields as undefined', () => {
      const datasource = createDatasource();
      const result = datasource.applyTemplateVariables(createQuery(), {});

      expect(result.groupId).toBeUndefined();
      expect(result.sensorPattern).toBeUndefined();
    });
  });
});
