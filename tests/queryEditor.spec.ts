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

  await panelEditPage.setVisualization('Table');
  await expect(panelEditPage.refreshPanel()).toBeOK();
  // The mock server's CPU Load / Total channel is a bounded [0, 100] percentage
  // (pkg/mockprtg/dataset.go's cpuChannels) -- not a fixed constant, so assert
  // a real value rendered rather than an exact number.
  await expect(panelEditPage.panel.fieldNames).toContainText(['Total']);
  await expect(panelEditPage.panel.data).not.toContainText(['No data']);
});
