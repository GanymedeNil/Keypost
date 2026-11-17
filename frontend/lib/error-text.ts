import { ApiError } from './api';
import type { TFunction } from './i18n/provider';

/** Localised, user-facing message for any thrown value. */
export function errorText(e: unknown, t: TFunction): string {
  if (e instanceof ApiError) {
    switch (e.code) {
      case 'forbidden':
        return t('errors.forbidden');
      case 'network_error':
        return t('errors.network');
      case 'rate_limited':
        return e.message || t('errors.rateLimited');
      case 'payload_too_large':
        return t('errors.tooLarge');
      default:
        return e.message || t('errors.generic');
    }
  }
  if (e instanceof Error && e.message) return e.message;
  return t('errors.generic');
}
