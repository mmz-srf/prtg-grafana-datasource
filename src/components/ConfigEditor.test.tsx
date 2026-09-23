import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DataSourceSettings } from '@grafana/data';
import { ConfigEditor } from './ConfigEditor';
import { PrtgDataSourceOptions, PrtgSecureJsonData } from '../types';

function createOptions(
  overrides: Partial<DataSourceSettings<PrtgDataSourceOptions, PrtgSecureJsonData>> = {}
): DataSourceSettings<PrtgDataSourceOptions, PrtgSecureJsonData> {
  return {
    id: 1,
    uid: 'test-uid',
    orgId: 1,
    name: 'PRTG',
    typeLogoUrl: '',
    type: 'swisstxt-prtg-datasource',
    typeName: 'PRTG',
    access: 'proxy',
    url: '',
    user: '',
    database: '',
    basicAuth: false,
    basicAuthUser: '',
    isDefault: false,
    jsonData: { authMode: 'apiKey' },
    secureJsonData: {},
    secureJsonFields: {},
    readOnly: false,
    withCredentials: false,
    ...overrides,
  };
}

describe('ConfigEditor', () => {
  it('renders the server URL and defaults to API key auth', () => {
    render(<ConfigEditor options={createOptions()} onOptionsChange={jest.fn()} />);

    expect(screen.getByLabelText('Server URL')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Enter your PRTG API key')).toBeInTheDocument();
    expect(screen.queryByLabelText('Username')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
  });

  it('switches to username/password fields and resets the API key secret', async () => {
    const onOptionsChange = jest.fn();
    const options = createOptions({ secureJsonFields: { apiKey: true } });
    render(<ConfigEditor options={options} onOptionsChange={onOptionsChange} />);

    await userEvent.click(screen.getByRole('radio', { name: 'Username & Password' }));

    expect(onOptionsChange).toHaveBeenCalledWith(
      expect.objectContaining({
        jsonData: expect.objectContaining({ authMode: 'credentials' }),
        secureJsonFields: expect.objectContaining({ apiKey: false }),
        secureJsonData: expect.objectContaining({ apiKey: '' }),
      })
    );
  });

  it('switches back to API key and resets the password secret', async () => {
    const onOptionsChange = jest.fn();
    const options = createOptions({
      jsonData: { authMode: 'credentials' },
      secureJsonFields: { password: true },
    });
    render(<ConfigEditor options={options} onOptionsChange={onOptionsChange} />);

    expect(screen.getByLabelText('Username')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('radio', { name: 'API Key' }));

    expect(onOptionsChange).toHaveBeenCalledWith(
      expect.objectContaining({
        jsonData: expect.objectContaining({ authMode: 'apiKey' }),
        secureJsonFields: expect.objectContaining({ password: false }),
        secureJsonData: expect.objectContaining({ password: '' }),
      })
    );
  });

  it('toggles TLS skip verify', async () => {
    const onOptionsChange = jest.fn();
    render(<ConfigEditor options={createOptions()} onOptionsChange={onOptionsChange} />);

    await userEvent.click(screen.getByRole('switch', { name: 'Skip TLS Verify' }));

    expect(onOptionsChange).toHaveBeenCalledWith(
      expect.objectContaining({ jsonData: expect.objectContaining({ tlsSkipVerify: true }) })
    );
  });

  it('toggles the secure socks proxy (Private Data Source Connect) switch', async () => {
    const onOptionsChange = jest.fn();
    render(<ConfigEditor options={createOptions()} onOptionsChange={onOptionsChange} />);

    await userEvent.click(screen.getByRole('switch', { name: 'Secure Socks Proxy' }));

    expect(onOptionsChange).toHaveBeenCalledWith(
      expect.objectContaining({ jsonData: expect.objectContaining({ enableSecureSocksProxy: true }) })
    );
  });
});
