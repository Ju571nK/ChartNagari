import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import { ChartGuide, ExperienceMode, loadUIMode, UI_MODE_KEY } from './ExperienceMode'
import i18n from './i18n'

beforeEach(() => localStorage.clear())
it('defaults new users to beginner and preserves explicit legacy preferences', () => {
  expect(loadUIMode()).toBe('beginner')
  localStorage.setItem(UI_MODE_KEY,'expert')
  expect(loadUIMode()).toBe('expert')
  localStorage.setItem(UI_MODE_KEY,'beginner')
  expect(loadUIMode()).toBe('beginner')
})
it.each(['en','ko','ja'])('provides keyboard-accessible mode and glossary in %s', async lang => {
  await i18n.changeLanguage(lang)
  const onChange = vi.fn()
  const user = userEvent.setup()
  const { container } = render(<><ExperienceMode mode="beginner" onChange={onChange} /><ChartGuide mode="beginner" /></>)
  const select = screen.getByRole('combobox', { name: i18n.t('experience.label') })
  await user.tab()
  expect(select).toHaveFocus()
  await user.selectOptions(select, 'expert')
  expect(onChange).toHaveBeenCalledWith('expert')
  await user.tab()
  expect(container.querySelector('summary')).toHaveFocus()
  // jsdom does not implement summary's native Enter default action.
  // Focusability is checked above; real-browser verification covers Enter.
  await user.click(container.querySelector('summary')!)
  expect(container.querySelector('details')).toHaveAttribute('open')
  expect(container.querySelectorAll('dt')).toHaveLength(9)
  expect(container.textContent).not.toContain('experience.')
})
