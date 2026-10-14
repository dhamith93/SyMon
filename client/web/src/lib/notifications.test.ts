import { describe, expect, it } from 'vitest';
import type { Channel } from './api';
import { deliveryState, describeChannel, describeReach, formFromChannel, inputFromForm, newChannelForm } from './notifications';

const email: Channel = {
  id: 1,
  name: 'ops email',
  kind: 'email',
  settings: { host: 'smtp.example.com', port: 587, user: 'symon', password: '', from: 'symon@example.com', to: ['ops@example.com', 'dev@example.com'] },
  secretsSet: ['password'],
  isDefault: true,
  skipWarnings: false,
  enabled: true,
  updatedAt: 1700000000,
  updatedBy: 'alice',
  lastStatus: 'sent',
  lastAt: 1700000100,
};

describe('the channel form', () => {
  it('turns a channel into text and back', () => {
    const form = formFromChannel(email);
    expect(form.values).toEqual({ host: 'smtp.example.com', port: '587', user: 'symon', from: 'symon@example.com', to: 'ops@example.com, dev@example.com' });
    expect(form.saved).toEqual(['password']);
    expect(inputFromForm(form)).toEqual({
      name: 'ops email',
      kind: 'email',
      // the empty password keeps the saved one
      settings: { host: 'smtp.example.com', port: 587, user: 'symon', password: '', from: 'symon@example.com', to: ['ops@example.com', 'dev@example.com'] },
      isDefault: true,
      skipWarnings: false,
      enabled: true,
    });
  });

  it('cleans up what was typed', () => {
    const form = newChannelForm('webhook');
    form.name = ' hook ';
    form.values = { url: 'https://hooks.example.com/x', headerName: ' Authorization ', headerValue: ' Bearer t ' };
    expect(inputFromForm(form).settings).toEqual({ url: 'https://hooks.example.com/x', headerName: 'Authorization', headerValue: ' Bearer t ' });
    expect(inputFromForm(form).name).toBe('hook');

    const numbered = newChannelForm('email');
    numbered.values = { port: 465 as unknown as string };
    expect(inputFromForm(numbered).settings.port).toBe(465);

    const mail = newChannelForm('email');
    mail.values = { host: 'smtp', port: 'abc', to: ' a@example.com,, b@example.com ,' };
    const settings = inputFromForm(mail).settings;
    expect(settings.to).toEqual(['a@example.com', 'b@example.com']);
    expect('port' in settings).toBe(false);
  });
});

describe('describing channels', () => {
  it('says where a channel sends and what it gets', () => {
    expect(describeChannel(email)).toBe('ops@example.com, dev@example.com through smtp.example.com');
    expect(describeChannel({ ...email, kind: 'slack', settings: { channel: '#ops' } })).toBe('#ops on Slack');
    expect(describeChannel({ ...email, kind: 'webhook', settings: {}, secretsSet: [] })).toBe('no URL yet');
    expect(describeReach(email)).toBe('Warnings and critical alerts, also from rules that pick no channel');
    expect(describeReach({ ...email, isDefault: false, skipWarnings: true })).toBe('Critical alerts from rules that pick it');
    expect(describeReach({ ...email, enabled: false })).toBe('Nothing, it is off');
  });

  it('tells sending, retrying and giving up apart', () => {
    expect(deliveryState('sent')).toEqual({ tone: 'good', label: 'Sent' });
    expect(deliveryState('pending')).toEqual({ tone: 'busy', label: 'Sending' });
    expect(deliveryState('pending', 'smtp is down')).toEqual({ tone: 'warning', label: 'Trying again' });
    expect(deliveryState('failed', 'gave up')).toEqual({ tone: 'bad', label: 'Failed' });
    expect(deliveryState('')).toEqual({ tone: 'busy', label: 'None yet' });
  });
});
