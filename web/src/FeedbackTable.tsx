// web/src/FeedbackTable.tsx
import { useTranslation } from 'react-i18next';
import type { FeedbackRow, FeedbackFilters } from './ExecutionTab';

type Props = {
	feedback: FeedbackRow[];
	filters: FeedbackFilters;
	onFiltersChange: (f: FeedbackFilters) => void;
	onRefresh: () => Promise<void>;
	pluginNames: string[];
};

const STATUSES = ['', 'SUBMITTED', 'FILLED', 'PARTIAL_FILL', 'REJECTED', 'CANCELLED', 'ERROR', 'RECEIVED'];

function statusClass(s: string): string {
	switch (s) {
		case 'FILLED':
		case 'PARTIAL_FILL': return 'status-filled';
		case 'REJECTED':
		case 'ERROR':        return 'status-rejected';
		case 'CANCELLED':    return 'status-cancelled';
		default:             return 'status-muted';
	}
}

export default function FeedbackTable({ feedback, filters, onFiltersChange, onRefresh, pluginNames }: Props) {
	const { t } = useTranslation();

	return (
		<div className="feedback-table">
			<div className="feedback-filters">
				<label>
					{t('execution.filter_plugin')}
					<select value={filters.plugin} onChange={e => onFiltersChange({ ...filters, plugin: e.target.value })}>
						<option value="">{t('common.all')}</option>
						{pluginNames.map(n => <option key={n} value={n}>{n}</option>)}
					</select>
				</label>
				<label>
					{t('execution.filter_status')}
					<select value={filters.status} onChange={e => onFiltersChange({ ...filters, status: e.target.value })}>
						{STATUSES.map(s => <option key={s || '_all'} value={s}>{s || t('common.all')}</option>)}
					</select>
				</label>
				<label>
					{t('execution.filter_symbol')}
					<input
						value={filters.symbol}
						onChange={e => onFiltersChange({ ...filters, symbol: e.target.value.toUpperCase() })}
					/>
				</label>
				<button onClick={() => void onRefresh()}>{t('common.refresh')}</button>
			</div>

			<table>
				<thead>
					<tr>
						<th>{t('execution.col_time')}</th>
						<th>{t('execution.col_plugin')}</th>
						<th>{t('execution.col_signal')}</th>
						<th>{t('execution.col_symbol')}</th>
						<th>{t('execution.col_status')}</th>
						<th>{t('execution.col_order')}</th>
						<th>{t('execution.col_message')}</th>
					</tr>
				</thead>
				<tbody>
					{feedback.length === 0 ? (
						<tr><td colSpan={7}>{t('execution.no_feedback')}</td></tr>
					) : feedback.map((r, i) => (
						<tr key={`${r.plugin_id}:${r.signal_id}:${r.order_id}:${r.status}:${i}`}>
							<td>{new Date(r.received_at * 1000).toISOString().replace('T', ' ').slice(0, 19)}</td>
							<td>{r.plugin_id}</td>
							<td><code>{r.signal_id.slice(0, 8)}</code></td>
							<td>{r.symbol || '—'}</td>
							<td className={statusClass(r.status)}>{r.status}</td>
							<td>{r.order_id || '—'}</td>
							<td>{r.message || '—'}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
}
