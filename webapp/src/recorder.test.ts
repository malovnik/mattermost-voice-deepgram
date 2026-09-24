import {describe, it, expect} from 'vitest';
import {durationLabel, extension, microphoneError} from './recorder';
describe('recorder formats and errors', () => {
    it('preserves Safari and Firefox containers', () => {
        expect(extension('audio/mp4;codecs=mp4a.40.2')).toBe('m4a');
        expect(extension('audio/ogg;codecs=opus')).toBe('ogg');
        expect(extension('audio/webm;codecs=opus')).toBe('webm');
    });
    it('gives actionable permission and device errors', () => {
        expect(microphoneError({name:'NotAllowedError'})).toContain('Разрешите');
        expect(microphoneError({name:'NotFoundError'})).toContain('не найден');
        expect(microphoneError({name:'NotReadableError'})).toContain('занят');
        expect(microphoneError(null)).toContain('Не удалось');
    });
    it('formats a five-minute recording', () => {
        expect(durationLabel(0)).toBe('0:00');
        expect(durationLabel(65)).toBe('1:05');
        expect(durationLabel(300)).toBe('5:00');
    });
});
