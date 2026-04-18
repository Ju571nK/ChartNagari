// web/src/FeedbackTable.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import FeedbackTable from './FeedbackTable';

const mockTranslations: Record<string, string> = {
	'execution.filter_plugin': 'Plugin',
	'execution.filter_status': 'Status',
	'execution.filter_symbol': 'Symbol',
	'execution.col_time': 'Time',
	'execution.col_plugin': 'Plugin',
	'execution.col_signal': 'Signal',
	'execution.col_symbol': 'Symbol',
	'execution.col_status': 'Status',
	'execution.col_order': 'Order',
	'execution.col_message': 'Message',
	'execution.no_feedback': 'No orders yet',
	'common.all': 'All',
	'common.refresh': 'Refresh',
};

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (k: string) => mockTranslations[k] ?? k }) }));
vi.mock('./i18n/index', () => ({ default: { language: 'en' } }));

const rows = [
	{ plugin_id: 'alpaca-paper', signal_id: '550e8400-e29b-41d4-a716-446655440000', order_id: 'o1', status: 'FILLED', symbol: 'AAPL', message: '', received_at: 1713312000 },
	{ plugin_id: 'alpaca-paper', signal_id: 'abc12345', order_id: 'o2', status: 'REJECTED', symbol: 'TSLA', message: 'denied', received_at: 1713311000 },
];

describe('FeedbackTable', () => {
	it('applies status color classes', () => {
		render(<FeedbackTable
			feedback={rows}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={vi.fn()}
			onRefresh={vi.fn()}
			pluginNames={['alpaca-paper']}
		/>);
		// The status values also appear in the <option> elements of the filter dropdown,
		// so use getAllByText and find the <td> cell specifically.
		const filledCells = screen.getAllByText('FILLED').filter(el => el.tagName === 'TD');
		expect(filledCells).toHaveLength(1);
		expect(filledCells[0]).toHaveClass('status-filled');

		const rejectedCells = screen.getAllByText('REJECTED').filter(el => el.tagName === 'TD');
		expect(rejectedCells).toHaveLength(1);
		expect(rejectedCells[0]).toHaveClass('status-rejected');
	});

	it('calls onFiltersChange synchronously when a dropdown changes', () => {
		const onFiltersChange = vi.fn();
		render(<FeedbackTable
			feedback={rows}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={onFiltersChange}
			onRefresh={vi.fn()}
			pluginNames={['alpaca-paper']}
		/>);
		fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'FILLED' } });
		expect(onFiltersChange).toHaveBeenCalledWith({ plugin: '', status: 'FILLED', symbol: '' });
	});

	it('Refresh button invokes onRefresh', () => {
		const onRefresh = vi.fn();
		render(<FeedbackTable
			feedback={[]}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={vi.fn()}
			onRefresh={onRefresh}
			pluginNames={[]}
		/>);
		fireEvent.click(screen.getByRole('button', { name: /refresh/i }));
		expect(onRefresh).toHaveBeenCalled();
	});

	it('renders empty-state row when feedback is empty', () => {
		render(<FeedbackTable
			feedback={[]}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={vi.fn()}
			onRefresh={vi.fn()}
			pluginNames={[]}
		/>);
		expect(screen.getByText(/no orders yet/i)).toBeInTheDocument();
	});

	it('uppercases symbol input on change', () => {
		const onFiltersChange = vi.fn();
		render(<FeedbackTable
			feedback={[]}
			filters={{ plugin: '', status: '', symbol: '' }}
			onFiltersChange={onFiltersChange}
			onRefresh={vi.fn()}
			pluginNames={[]}
		/>);
		fireEvent.change(screen.getByLabelText(/symbol/i), { target: { value: 'aapl' } });
		expect(onFiltersChange).toHaveBeenCalledWith({ plugin: '', status: '', symbol: 'AAPL' });
	});
});
