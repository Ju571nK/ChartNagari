import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import PluginCard from './PluginCard';

const mockTranslations: Record<string, string> = {
	'execution.no_activity': 'no activity',
	'execution.confirm_delete_plugin': 'Delete plugin {{name}}?',
	'execution.add_plugin': '+ Add plugin',
	'common.edit': 'Edit',
	'common.delete': 'Delete',
	'common.cancel': 'Cancel',
	'common.confirm': 'Confirm',
};
vi.mock('react-i18next', () => ({
	useTranslation: () => ({
		t: (k: string, opts?: Record<string, string>) => {
			let val = mockTranslations[k] ?? k;
			if (opts) {
				Object.entries(opts).forEach(([key, replacement]) => {
					val = val.replace(`{{${key}}}`, replacement);
				});
			}
			return val;
		},
	}),
}));
vi.mock('./i18n/index', () => ({ default: { language: 'en' } }));

const basePlugin = {
	id: 'alpaca-paper',
	url: 'http://localhost:9100',
	enabled: true,
	symbols: [],
	min_score: 12,
	direction_filter: '' as const,
	secret: '••••',
};

describe('PluginCard', () => {
	it('shows "no activity" when stats are undefined', () => {
		render(<PluginCard plugin={basePlugin} stats={undefined} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(screen.getByText(/no activity/i)).toBeInTheDocument();
	});

	it('renders 24h counts when stats are provided', () => {
		const stats = { plugin_id: 'alpaca-paper', submitted: 13, filled: 12, rejected: 1, last_failure_msg: '' };
		render(<PluginCard plugin={basePlugin} stats={stats} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(screen.getByText(/12 filled/i)).toBeInTheDocument();
		expect(screen.getByText(/1 rejected/i)).toBeInTheDocument();
	});

	it('shows the danger border when last_failure_msg is present', () => {
		const stats = { plugin_id: 'alpaca-paper', submitted: 1, filled: 0, rejected: 1, last_failure_msg: 'denied' };
		const { container } = render(<PluginCard plugin={basePlugin} stats={stats} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(container.firstChild).toHaveClass('plugin-card--has-failure');
	});

	it('renders disabled class when enabled=false', () => {
		const plugin = { ...basePlugin, enabled: false };
		const { container } = render(<PluginCard plugin={plugin} stats={undefined} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />);
		expect(container.firstChild).toHaveClass('plugin-card--disabled');
	});

	it('calls onDelete only after confirming', () => {
		const onDelete = vi.fn();
		render(<PluginCard plugin={basePlugin} stats={undefined} onEdit={vi.fn()} onDelete={onDelete} onToggleEnabled={vi.fn()} />);
		fireEvent.click(screen.getByRole('button', { name: /delete/i }));
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
		expect(onDelete).not.toHaveBeenCalled();

		fireEvent.click(screen.getByRole('button', { name: /delete/i }));
		fireEvent.click(screen.getByRole('button', { name: /^confirm$/i }));
		expect(onDelete).toHaveBeenCalledTimes(1);
	});
});
