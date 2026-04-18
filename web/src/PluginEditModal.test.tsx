import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import PluginEditModal from './PluginEditModal';
import type { Plugin } from './ExecutionTab';

// i18n mock — same pattern as KillSwitch/PluginCard tests.
vi.mock('react-i18next', () => ({
	useTranslation: () => ({
		t: (k: string, opts?: { name?: string }) => {
			const map: Record<string, string> = {
				'execution.field_name': 'Name',
				'execution.field_url': 'URL',
				'execution.field_secret': 'Secret',
				'execution.field_min_score': 'Min score',
				'execution.field_symbols': 'Symbols (comma-separated)',
				'execution.field_direction': 'Direction filter',
				'execution.field_enabled': 'Enabled',
				'execution.secret_placeholder': 'Leave blank to keep current secret',
				'execution.secret_copied': 'Secret copied to clipboard',
				'execution.secret_copy_manual': 'Copy the secret manually before saving',
				'execution.generate': 'Generate',
				'execution.err_name_required': 'Name is required',
				'execution.err_name_exists': 'already exists',
				'execution.err_url': 'must start with http:// or https://',
				'execution.err_min_score': 'must be ≥ 0',
				'execution.err_secret_required': 'Secret is required for new plugins',
				'common.show': 'Show',
				'common.hide': 'Hide',
				'common.both': 'Both',
				'common.cancel': 'Cancel',
				'common.save': 'Save',
			};
			if (k === 'execution.confirm_delete_plugin' && opts?.name) return `Delete plugin ${opts.name}?`;
			return map[k] ?? k;
		},
	}),
}));

const nav = globalThis.navigator as any;

beforeEach(() => {
	nav.clipboard = { writeText: vi.fn().mockResolvedValue(undefined) };
	(globalThis.crypto as any).getRandomValues = (arr: Uint8Array) => {
		for (let i = 0; i < arr.length; i++) arr[i] = i;
		return arr;
	};
});

const full: Plugin = {
	id: 'a', url: 'http://a', enabled: true, symbols: [], min_score: 1, direction_filter: '', secret: '••••',
};

describe('PluginEditModal', () => {
	it('requires name and secret in create mode', () => {
		const onSave = vi.fn();
		render(<PluginEditModal plugin={null} existingIds={[]} onSave={onSave} onCancel={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /save/i }));
		expect(onSave).not.toHaveBeenCalled();
		expect(screen.getByText(/name is required/i)).toBeInTheDocument();
	});

	it('rejects duplicate name in create mode', () => {
		render(<PluginEditModal plugin={null} existingIds={['alpaca-paper']} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.change(screen.getByLabelText(/^name/i), { target: { value: 'alpaca-paper' } });
		fireEvent.blur(screen.getByLabelText(/^name/i));
		expect(screen.getByText(/already exists/i)).toBeInTheDocument();
	});

	it('rejects non-http URL', () => {
		render(<PluginEditModal plugin={null} existingIds={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		const url = screen.getByLabelText(/url/i);
		fireEvent.change(url, { target: { value: 'ftp://x' } });
		fireEvent.blur(url);
		expect(screen.getByText(/must start with http/i)).toBeInTheDocument();
	});

	it('rejects negative min_score', () => {
		render(<PluginEditModal plugin={null} existingIds={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		const score = screen.getByLabelText(/min score/i);
		fireEvent.change(score, { target: { value: '-1' } });
		fireEvent.blur(score);
		expect(screen.getByText(/must be ≥ 0/i)).toBeInTheDocument();
	});

	it('Generate fills secret field and copies to clipboard', async () => {
		render(<PluginEditModal plugin={null} existingIds={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /generate/i }));
		await screen.findByText(/copied to clipboard/i);
		expect(nav.clipboard.writeText).toHaveBeenCalled();
		const input = screen.getByLabelText(/^secret/i) as HTMLInputElement;
		expect(input.value.length).toBe(64);
	});

	it('Generate falls back with manual-copy toast when clipboard rejects', async () => {
		nav.clipboard.writeText = vi.fn().mockRejectedValue(new Error('no https'));
		render(<PluginEditModal plugin={null} existingIds={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /generate/i }));
		await waitFor(() => expect(screen.getByText(/copy the secret manually/i)).toBeInTheDocument());
	});

	it('resets form when plugin prop changes (useEffect pattern)', () => {
		const { rerender } = render(<PluginEditModal plugin={full} existingIds={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		expect((screen.getByLabelText(/url/i) as HTMLInputElement).value).toBe('http://a');
		rerender(<PluginEditModal plugin={{ ...full, id: 'b', url: 'http://b' }} existingIds={[]} onSave={vi.fn()} onCancel={vi.fn()} />);
		expect((screen.getByLabelText(/url/i) as HTMLInputElement).value).toBe('http://b');
	});
});
