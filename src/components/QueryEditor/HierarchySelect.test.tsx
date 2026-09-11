import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HierarchySelect } from './HierarchySelect';
import { HierarchyItem } from './toSelectableValue';

// jsdom has no IntersectionObserver, which @grafana/ui's menu/scroll
// indicators use when the select's menu opens.
class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null = null;
  readonly rootMargin: string = '';
  readonly thresholds: readonly number[] = [];
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }
}
global.IntersectionObserver = MockIntersectionObserver;

function renderSelect(overrides: {
  value?: string;
  valueName?: string;
  fetcher?: () => Promise<HierarchyItem[]>;
  onChange?: (id?: string, name?: string) => void;
} = {}) {
  const onChange = overrides.onChange ?? jest.fn();
  const fetcher = overrides.fetcher ?? jest.fn().mockResolvedValue([{ id: 'g1', name: 'Group One' }]);
  render(
    <HierarchySelect
      inputId="test-select"
      aria-label="Test select"
      value={overrides.value}
      valueName={overrides.valueName}
      onChange={onChange}
      fetcher={fetcher}
    />
  );
  return { onChange, fetcher };
}

describe('HierarchySelect', () => {
  it('calls the fetcher on mount', () => {
    const { fetcher } = renderSelect();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("shows the saved display name before the async options have loaded", () => {
    const fetcher = jest.fn(() => new Promise<HierarchyItem[]>(() => {}));
    renderSelect({ value: 'g1', valueName: 'Saved Group', fetcher });

    expect(screen.getByText('Saved Group')).toBeInTheDocument();
  });

  it('lets a user type a custom value (e.g. a template variable) and calls onChange with it', async () => {
    const { onChange } = renderSelect({ fetcher: jest.fn().mockResolvedValue([]) });

    const input = screen.getByLabelText('Test select');
    await userEvent.type(input, '$myVar');
    await userEvent.keyboard('{Enter}');

    expect(onChange).toHaveBeenCalledWith('$myVar', '$myVar');
  });

  it('calls onChange with undefined when the selection is cleared via backspace', async () => {
    const { onChange } = renderSelect({
      value: 'g1',
      valueName: 'Group One',
      fetcher: jest.fn().mockResolvedValue([{ id: 'g1', name: 'Group One' }]),
    });

    const input = await screen.findByLabelText('Test select');
    await userEvent.click(input);
    await userEvent.keyboard('{Backspace}');

    expect(onChange).toHaveBeenCalledWith(undefined, undefined);
  });
});
