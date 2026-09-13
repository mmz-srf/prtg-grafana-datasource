import { test, expect } from '@grafana/plugin-e2e';
import { PrtgDataSourceOptions, PrtgSecureJsonData } from '../src/types';

test('smoke: should render config editor', async ({ createDataSourceConfigPage, readProvisionedDataSource, page }) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await createDataSourceConfigPage({ type: ds.type });
  await expect(page.getByRole('textbox', { name: 'Server URL' })).toBeVisible();
});

test('"Save & test" should be successful when configuration is valid', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource<PrtgDataSourceOptions, PrtgSecureJsonData>({ fileName: 'datasources.yml' });
  const configPage = await createDataSourceConfigPage({ type: ds.type });
  await page.getByRole('textbox', { name: 'Server URL' }).fill(ds.jsonData.serverUrl ?? '');
  await page.getByRole('textbox', { name: 'API Key' }).fill(ds.secureJsonData?.apiKey ?? '');
  await expect(configPage.saveAndTest()).toBeOK();
});

test('"Save & test" should fail when configuration is invalid', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource<PrtgDataSourceOptions, PrtgSecureJsonData>({ fileName: 'datasources.yml' });
  const configPage = await createDataSourceConfigPage({ type: ds.type });
  // Server URL only, no API Key -- a fresh config page defaults to authMode
  // 'apiKey' (ConfigEditor.tsx), matching pkg/models/settings.go's Validate().
  await page.getByRole('textbox', { name: 'Server URL' }).fill(ds.jsonData.serverUrl ?? '');
  await expect(configPage.saveAndTest()).not.toBeOK();
  await expect(configPage).toHaveAlert('error', { hasText: 'API key is required' });
});
