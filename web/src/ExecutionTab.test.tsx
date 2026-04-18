import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import ExecutionTab from './ExecutionTab';

beforeEach(() => {
	globalThis.fetch = vi.fn().mockImplementation((url: string | URL) => {
		const u = typeof url === 'string' ? url : url.toString();
		if (u.includes('/api/execution/config')) {
			return Promise.resolve(new Response(JSON.stringify({
				version: 1, enabled: false, killed_at: '', plugins: [], max_dispatched: 3, dedup_window_sec: 300, symbol_map: {},
			})));
		}
		if (u.includes('/api/execution/plugins/stats')) {
			return Promise.resolve(new Response(JSON.stringify({ window: '24h', plugins: [] })));
		}
		if (u.includes('/api/execution/feedback')) {
			return Promise.resolve(new Response(JSON.stringify({ items: [], count: 0 })));
		}
		return Promise.resolve(new Response('{}'));
	});
});

describe('ExecutionTab', () => {
	it('fetches config, stats, and feedback in parallel on mount', async () => {
		render(<ExecutionTab />);
		await waitFor(() => {
			expect(globalThis.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/execution/config'), expect.anything());
			expect(globalThis.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/execution/plugins/stats'), expect.anything());
			expect(globalThis.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/execution/feedback'), expect.anything());
		});
	});

	it('renders the kill switch, plugin area, global form, and feedback table slots', async () => {
		render(<ExecutionTab />);
		await waitFor(() => {
			expect(screen.getByTestId('kill-switch')).toBeInTheDocument();
			expect(screen.getByTestId('plugins-area')).toBeInTheDocument();
			expect(screen.getByTestId('global-config')).toBeInTheDocument();
			expect(screen.getByTestId('feedback-table')).toBeInTheDocument();
		});
	});
});
