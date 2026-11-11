import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import i18n from './i18n';
import OrderPluginGuide from './OrderPluginGuide';

afterEach(async () => {
  cleanup();
  vi.unstubAllGlobals();
  await i18n.changeLanguage('en');
});

describe('OrderPluginGuide', () => {
  it.each([
    ['en', 'Order plugin developer kit', 'not an install-and-trade', 'Who is this for, and how do I start?', 'en.md'],
    ['ko', '주문 플러그인 개발키트 안내', '바로 실계좌로 주문하는 기능은 아직', '누가 사용하나요? 무엇부터 시작하나요?', 'ko.md'],
    ['ja', '注文プラグイン開発キットのご案内', 'すぐに実口座で注文できる機能ではありません', '対象ユーザーと始め方を確認する', 'ja.md'],
  ])('explains scope and opens localized guidance in %s without API calls', async (language, heading, warning, summary, path) => {
    await i18n.changeLanguage(language);
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    render(<OrderPluginGuide />);

    expect(screen.getByRole('region', { name: heading })).toBeInTheDocument();
    expect(screen.getByText(new RegExp(warning))).toBeVisible();
    const disclosure = screen.getByText(summary);
    expect(disclosure.closest('details')).not.toHaveAttribute('open');
    fireEvent.click(disclosure);
    expect(disclosure.closest('details')).toHaveAttribute('open');
    expect(screen.getAllByRole('listitem')).toHaveLength(4);
    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute('href', 'https://github.com/Ju571nK/ChartNagari/blob/main/docs/order-plugins/' + path);
    expect(links[1]).toHaveAttribute('href', 'https://github.com/Ju571nK/ChartNagari/blob/main/docs/order-plugins/protocol.md');
    for (const link of links) {
      expect(link).toHaveAttribute('target', '_blank');
      expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    }
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
    fireEvent.click(disclosure);
    expect(disclosure.closest('details')).not.toHaveAttribute('open');
  });
});
