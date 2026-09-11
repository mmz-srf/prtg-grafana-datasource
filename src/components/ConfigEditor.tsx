import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps, SelectableValue } from '@grafana/data';
import { Alert, InlineField, Input, InlineSwitch, RadioButtonGroup, SecretInput, Stack } from '@grafana/ui';
import { PrtgAuthMode, PrtgDataSourceOptions, PrtgSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<PrtgDataSourceOptions, PrtgSecureJsonData> {}

const AUTH_MODE_OPTIONS: Array<SelectableValue<PrtgAuthMode>> = [
  { label: 'API Key', value: 'apiKey' },
  { label: 'Username & Password', value: 'credentials' },
];

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;
  const authMode: PrtgAuthMode = jsonData.authMode ?? 'apiKey';

  const onServerUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: { ...jsonData, serverUrl: event.target.value },
    });
  };

  const onAuthModeChange = (value: PrtgAuthMode) => {
    // Reset the *other* mode's secrets so a stale, unused secret isn't left configured.
    onOptionsChange({
      ...options,
      jsonData: { ...jsonData, authMode: value },
      secureJsonFields: {
        ...secureJsonFields,
        apiKey: value === 'apiKey' ? secureJsonFields?.apiKey : false,
        password: value === 'credentials' ? secureJsonFields?.password : false,
      },
      secureJsonData: {
        ...secureJsonData,
        apiKey: value === 'apiKey' ? secureJsonData?.apiKey : '',
        password: value === 'credentials' ? secureJsonData?.password : '',
      },
    });
  };

  const onUsernameChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: { ...jsonData, username: event.target.value },
    });
  };

  const onTlsSkipVerifyChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: { ...jsonData, tlsSkipVerify: event.target.checked },
    });
  };

  const onApiKeyChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: { ...secureJsonData, apiKey: event.target.value },
    });
  };

  const onResetApiKey = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: { ...secureJsonFields, apiKey: false },
      secureJsonData: { ...secureJsonData, apiKey: '' },
    });
  };

  const onPasswordChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: { ...secureJsonData, password: event.target.value },
    });
  };

  const onResetPassword = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: { ...secureJsonFields, password: false },
      secureJsonData: { ...secureJsonData, password: '' },
    });
  };

  return (
    <Stack direction="column" gap={2}>
      <Alert severity="info" title="PRTG API v2 required">
        This datasource talks to PRTG&apos;s API v2 (<code>/api/v2/...</code>), which requires the PRTG
        &quot;Application Server&quot; and the new UI/API to be activated on the target PRTG core server (Setup{' '}
        &rarr; Activate New UI And New API). Every API v2 endpoint this plugin depends on is currently marked
        &quot;Experimental&quot; by Paessler and may change between PRTG releases.
      </Alert>
      <InlineField label="Server URL" labelWidth={25} interactive tooltip="The base URL of your PRTG server">
        <Input
          id="config-editor-server-url"
          onChange={onServerUrlChange}
          value={jsonData.serverUrl ?? ''}
          placeholder="https://prtg.example.com"
          width={80}
        />
      </InlineField>
      <InlineField
        label="Authentication"
        labelWidth={25}
        interactive
        tooltip="How the plugin authenticates against the PRTG API"
      >
        <RadioButtonGroup<PrtgAuthMode>
          id="config-editor-auth-mode"
          options={AUTH_MODE_OPTIONS}
          value={authMode}
          onChange={onAuthModeChange}
        />
      </InlineField>
      {authMode === 'apiKey' ? (
        <InlineField label="API Key" labelWidth={25} interactive tooltip="Secure json field (backend only)">
          <SecretInput
            required
            id="config-editor-api-key"
            isConfigured={secureJsonFields?.apiKey ?? false}
            value={secureJsonData?.apiKey ?? ''}
            placeholder="Enter your PRTG API key"
            width={80}
            onReset={onResetApiKey}
            onChange={onApiKeyChange}
          />
        </InlineField>
      ) : (
        <>
          <InlineField label="Username" labelWidth={25} interactive tooltip="PRTG username">
            <Input
              id="config-editor-username"
              onChange={onUsernameChange}
              value={jsonData.username ?? ''}
              placeholder="Enter your PRTG username"
              width={80}
            />
          </InlineField>
          <InlineField label="Password" labelWidth={25} interactive tooltip="Secure json field (backend only)">
            <SecretInput
              required
              id="config-editor-password"
              isConfigured={secureJsonFields?.password ?? false}
              value={secureJsonData?.password ?? ''}
              placeholder="Enter your PRTG password"
              width={80}
              onReset={onResetPassword}
              onChange={onPasswordChange}
            />
          </InlineField>
        </>
      )}
      <InlineField
        label="Skip TLS Verify"
        labelWidth={25}
        interactive
        tooltip="Skip TLS certificate verification when connecting to the PRTG server"
      >
        <InlineSwitch
          id="config-editor-tls-skip-verify"
          value={jsonData.tlsSkipVerify ?? false}
          onChange={onTlsSkipVerifyChange}
        />
      </InlineField>
    </Stack>
  );
}
