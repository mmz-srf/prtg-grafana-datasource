import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor/QueryEditor';
import { PrtgQuery, PrtgDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, PrtgQuery, PrtgDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
