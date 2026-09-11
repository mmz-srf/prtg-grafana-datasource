import { useCallback, useEffect, useState } from 'react';
import { SelectableValue } from '@grafana/data';
import { HierarchyItem, toSelectableValues } from './toSelectableValue';

interface UseResourceOptionsResult {
  options: Array<SelectableValue<string>>;
  loading: boolean;
  error: unknown;
  /** Suitable as an `AsyncSelect` `loadOptions`: filters the loaded list client-side by label. */
  loadOptions: (query: string) => Promise<Array<SelectableValue<string>>>;
}

/**
 * Loads a PRTG hierarchy resource (group/device/sensor/channel list) via the
 * given `fetcher` whenever it changes (callers should memoize it, e.g. with
 * `useCallback`, so it only changes when its real inputs — like a parent id —
 * change; the datasource's own Promise-cache dedupes the underlying network
 * calls regardless).
 */
export function useResourceOptions(fetcher: () => Promise<HierarchyItem[]>): UseResourceOptionsResult {
  const [options, setOptions] = useState<Array<SelectableValue<string>>>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(undefined);

  useEffect(() => {
    let cancelled = false;
    // Intentional: this is the standard data-fetching-in-an-effect pattern
    // (https://react.dev/learn/synchronizing-with-effects#fetching-data) —
    // reset to a loading state synchronously so the UI reflects the new
    // fetcher immediately, then resolve it asynchronously below.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    setError(undefined);

    fetcher()
      .then((items) => {
        if (!cancelled) {
          setOptions(toSelectableValues(items));
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err);
          setOptions([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [fetcher]);

  const loadOptions = useCallback(
    async (query: string) => {
      if (!query) {
        return options;
      }
      const lowerQuery = query.toLowerCase();
      return options.filter((option) => (option.label ?? '').toLowerCase().includes(lowerQuery));
    },
    [options]
  );

  return { options, loading, error, loadOptions };
}
