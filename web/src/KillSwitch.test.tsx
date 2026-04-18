import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import KillSwitch from './KillSwitch';

const mockTranslations: Record<string, string> = {
	'execution.kill_switch': 'Kill Switch',
	'execution.reenable': 'Re-enable',
	'execution.killed_banner': 'EXECUTION KILLED — no signals being dispatched',
	'execution.last_killed': 'Last killed',
	'execution.confirm_kill': 'Really disable all plugin dispatch?',
	'execution.confirm_reenable': 'Really re-enable all plugin dispatch?',
	'common.cancel': 'Cancel',
	'common.confirm': 'Confirm',
};
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (k: string) => mockTranslations[k] ?? k }) }));
vi.mock('./i18n/index', () => ({ default: { language: 'en' } }));

describe('KillSwitch', () => {
	it('renders the Kill button when not killed', () => {
		render(<KillSwitch killed={false} killedAt={null} onToggle={vi.fn()} />);
		expect(screen.getByRole('button', { name: /kill/i })).toBeInTheDocument();
		expect(screen.queryByRole('banner')).toBeNull();
	});

	it('renders red banner with formatted timestamp when killed', () => {
		render(<KillSwitch killed={true} killedAt="2026-04-17T10:30:00Z" onToggle={vi.fn()} />);
		expect(screen.getByRole('banner')).toHaveTextContent(/execution killed/i);
		expect(screen.getByRole('banner')).toHaveTextContent(/2026/);
		expect(screen.getByRole('button', { name: /re-enable/i })).toBeInTheDocument();
	});

	it('opens confirm modal on click and calls onToggle only on Confirm', async () => {
		const onToggle = vi.fn().mockResolvedValue(undefined);
		render(<KillSwitch killed={false} killedAt={null} onToggle={onToggle} />);

		fireEvent.click(screen.getByRole('button', { name: /kill/i }));
		expect(screen.getByRole('dialog')).toBeInTheDocument();

		fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
		expect(onToggle).not.toHaveBeenCalled();

		fireEvent.click(screen.getByRole('button', { name: /kill/i }));
		fireEvent.click(screen.getByRole('button', { name: /^confirm$/i }));
		expect(onToggle).toHaveBeenCalledTimes(1);
	});
});
