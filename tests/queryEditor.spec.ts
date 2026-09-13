import { test, expect } from '@grafana/plugin-e2e';

// Selection chain from pkg/mockprtg/dataset.go's seeded topology: a group
// with two devices, one of which ("core-router-01") has a "CPU Load" sensor
// with "Total"/"5 Min Avg" channels.
const GROUP = 'Core Infrastructure';
const DEVICE = 'core-router-01';
const SENSOR = 'CPU Load';
const CHANNEL = 'Total';

test('smoke: should render query editor', async ({ panelEditPage, readProvisionedDataSource }) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);

  const row = panelEditPage.getQueryEditorRow('A');
  await expect(row.getByRole('radio', { name: 'Current value' })).toBeVisible();
  await expect(row.getByRole('combobox', { name: 'Group' })).toBeVisible();
  await expect(row.getByRole('combobox', { name: 'Device' })).toBeVisible();
  await expect(row.getByRole('combobox', { name: 'Sensor' })).toBeVisible();
  // Channel starts disabled (no sensor picked yet), so it has no combobox
  // role until then -- just its placeholder text.
  await expect(row.getByText('Select a sensor first')).toBeVisible();
});

test('picking a full group/device/sensor/channel selection triggers a new query', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);
  const row = panelEditPage.getQueryEditorRow('A');

  await row.getByRole('combobox', { name: 'Group' }).click();
  await page.getByText(GROUP, { exact: true }).click();
  await row.getByRole('combobox', { name: 'Device' }).click();
  await page.getByText(DEVICE, { exact: true }).click();
  await row.getByRole('combobox', { name: 'Sensor' }).click();
  await page.getByText(SENSOR, { exact: true }).click();

  const queryReq = panelEditPage.waitForQueryDataRequest();
  await row.getByRole('combobox', { name: 'Channel' }).click();
  await page.getByText(CHANNEL, { exact: true }).click();
  await expect(await queryReq).toBeTruthy();
});

test('current value query returns real data for the selected channel', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);
  const row = panelEditPage.getQueryEditorRow('A');

  await row.getByRole('combobox', { name: 'Group' }).click();
  await page.getByText(GROUP, { exact: true }).click();
  await row.getByRole('combobox', { name: 'Device' }).click();
  await page.getByText(DEVICE, { exact: true }).click();
  await row.getByRole('combobox', { name: 'Sensor' }).click();
  await page.getByText(SENSOR, { exact: true }).click();
  await row.getByRole('combobox', { name: 'Channel' }).click();
  await page.getByText(CHANNEL, { exact: true }).click();

  // Inspect the raw /api/ds/query response instead of a rendered
  // visualization: setVisualization()'s viz-picker flow differs across
  // Grafana versions (see @grafana/plugin-e2e's version.gte(..., '12.4.0')
  // branch) and is flaky in CI against older supported versions -- reading
  // the response body directly is both more robust and a more direct check
  // that the datasource actually returned data.
  const response = await panelEditPage.refreshPanel();
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  const frames = body.results?.A?.frames;
  expect(Array.isArray(frames) && frames.length > 0).toBe(true);

  // The mock server's CPU Load / Total channel is a bounded [0, 100]
  // percentage (pkg/mockprtg/dataset.go's cpuChannels) -- not a fixed
  // constant, so assert a real numeric value came back rather than an exact
  // number. data.values is [times[], values[]] for a "current value" query.
  const values = frames[0]?.data?.values?.[1];
  expect(Array.isArray(values) && values.length > 0).toBe(true);
  expect(typeof values[0]).toBe('number');
});
