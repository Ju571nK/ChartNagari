import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import GlobalConfigForm from './GlobalConfigForm';

vi.mock('react-i18next', () => ({
	useTranslation: () => ({
		t: (k: string) => {
			const map: Record<string, string> = {
				'execution.max_dispatched': 'Max dispatched',
				'execution.dedup_window': 'Dedup window (seconds)',
				'execution.symbol_map': 'Symbol map (advanced)',
				'execution.add_row': 'Add row',
				'execution.remove_row': 'Remove row',
				'common.discard': 'Discard',
				'common.save': 'Save',
			};
			return map[k] ?? k;
		},
	}),
}));

const base = {
	max_dispatched: 3,
	dedup_window_sec: 300,
	symbol_map: { BTCUSDT: 'BTC/USD' },
};

describe('GlobalConfigForm', () => {
	it('Save is disabled when no changes', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		expect(screen.getByRole('button', { name: /save/i })).toBeDisabled();
	});

	it('enables Save on any change', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		fireEvent.change(screen.getByLabelText(/max dispatched/i), { target: { value: '5' } });
		expect(screen.getByRole('button', { name: /save/i })).toBeEnabled();
	});

	it('Add row appends a blank symbol row', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		const before = screen.getAllByLabelText(/^symbol$/i).length;
		fireEvent.click(screen.getByRole('button', { name: /add row/i }));
		const after = screen.getAllByLabelText(/^symbol$/i).length;
		expect(after).toBe(before + 1);
	});

	it('Remove row deletes a row', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		const before = screen.getAllByLabelText(/^symbol$/i).length;
		fireEvent.click(screen.getAllByRole('button', { name: /remove row/i })[0]);
		const after = screen.queryAllByLabelText(/^symbol$/i).length;
		expect(after).toBe(before - 1);
	});

	it('Discard resets form to config', () => {
		render(<GlobalConfigForm config={base} onSave={vi.fn()} onServerError={null} />);
		fireEvent.change(screen.getByLabelText(/max dispatched/i), { target: { value: '5' } });
		fireEvent.click(screen.getByRole('button', { name: /discard/i }));
		expect((screen.getByLabelText(/max dispatched/i) as HTMLInputElement).value).toBe('3');
	});

	it('renders with undefined symbol_map without crashing', () => {
		const configNoMap = { max_dispatched: 0, dedup_window_sec: 300, symbol_map: undefined };
		render(<GlobalConfigForm config={configNoMap} onSave={vi.fn()} onServerError={null} />);
		expect(screen.getByRole('button', { name: /save/i })).toBeDisabled();
	});
});
