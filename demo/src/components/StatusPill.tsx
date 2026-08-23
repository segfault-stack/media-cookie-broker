import type {CSSProperties, ReactNode} from 'react';
import {colors, sans} from '../theme';

type Tone = 'healthy' | 'attention' | 'neutral' | 'browser';

const toneStyles: Record<Tone, CSSProperties> = {
  healthy: {color: '#a9efbd', background: colors.greenBg, borderColor: '#2b7043'},
  attention: {color: '#ffd398', background: colors.orangeBg, borderColor: '#865b2a'},
  neutral: {color: '#c8d3de', background: '#202b37', borderColor: '#445466'},
  browser: {color: '#c5e0ff', background: colors.blueBg, borderColor: '#366c9f'},
};

export const StatusPill: React.FC<{children: ReactNode; tone: Tone; style?: CSSProperties}> = ({children, tone, style}) => (
  <div style={{display: 'inline-flex', alignItems: 'center', gap: 9, minHeight: 34, padding: '6px 12px', border: '1px solid', borderRadius: 999, fontFamily: sans, fontSize: 15, fontWeight: 700, letterSpacing: -0.1, ...toneStyles[tone], ...style}}>
    <span style={{width: 8, height: 8, borderRadius: 8, background: tone === 'healthy' ? colors.green : tone === 'attention' ? colors.orange : tone === 'browser' ? colors.blue : colors.muted, boxShadow: `0 0 12px ${tone === 'healthy' ? colors.green : tone === 'attention' ? colors.orange : colors.blue}66`}} />
    {children}
  </div>
);
