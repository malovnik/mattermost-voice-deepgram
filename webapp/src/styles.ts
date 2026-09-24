export const styles = `
.vd-overlay{position:fixed;inset:0;z-index:10000;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.5);padding:16px}
.vd-dialog{box-sizing:border-box;width:460px;max-width:100%;max-height:90vh;overflow:auto;border-radius:12px;background:var(--center-channel-bg,#fff);color:var(--center-channel-color,#303846);padding:24px;box-shadow:0 16px 50px #0004;outline:none}
.vd-header{display:flex;align-items:center;justify-content:space-between;gap:12px}.vd-header h2{font-size:22px;margin:0;font-weight:600}.vd-header button{font-size:24px;border:0!important}
.vd-dialog button,.vd-toast button{cursor:pointer;border:1px solid var(--center-channel-color-24,#bcc1ca);border-radius:5px;padding:9px 14px;background:transparent;color:inherit;font-size:14px;font-weight:600}
.vd-dialog button:disabled{opacity:.5;cursor:not-allowed}.vd-dialog button:focus-visible,.vd-dialog a:focus-visible{outline:3px solid var(--button-bg,#166de0);outline-offset:3px}
.vd-dialog .vd-primary{background:var(--button-bg,#166de0);border-color:var(--button-bg,#166de0);color:var(--button-color,#fff)}
.vd-muted{opacity:.75;font-size:14px;margin:12px 0}.vd-clock{text-align:center;font-size:54px;line-height:1.2;font-variant-numeric:tabular-nums;margin:26px 0 8px}.vd-clock+.vd-muted{text-align:center}.vd-live{color:var(--error-text,#cf3737)}
.vd-dialog audio{width:100%;margin:12px 0}.vd-actions{display:flex;flex-wrap:wrap;gap:10px;margin:20px 0 14px}.vd-footnote{font-size:12px;line-height:1.6;opacity:.7;margin-top:20px}.vd-error{color:var(--error-text,#be3636)}.vd-notice{background:var(--center-channel-color-08,#eef1f6);padding:12px;border-radius:6px;line-height:1.5;font-size:13px}
.vd-toast{position:fixed;bottom:28px;left:50%;transform:translateX(-50%);z-index:10001;max-width:90vw;padding:16px 20px;background:var(--center-channel-bg,#fff);color:var(--center-channel-color,#303846);box-shadow:0 5px 24px #0004;border:1px solid #8885;border-radius:8px;display:flex;align-items:center;gap:16px}
`;
