import type {ReactNode} from 'react';
import {colors, sans} from '../theme';

export const BrowserWindow: React.FC<{children: ReactNode; incognito?: boolean; title?: string}> = ({children, incognito = false, title = 'Extension · Media Cookie Broker'}) => (
  <div style={{width: '100%', height: '100%', overflow: 'hidden', borderRadius: 18, border: `1px solid ${incognito ? '#5e5872' : colors.border}`, background: '#111820', boxShadow: '0 24px 70px #0008'}}>
    <div style={{height: 50, display: 'flex', alignItems: 'center', padding: '0 17px', gap: 9, background: incognito ? '#292634' : '#202a35', borderBottom: `1px solid ${incognito ? '#514b62' : colors.border}`, fontFamily: sans, color: colors.muted, fontSize: 13}}>
      <span style={{width: 11, height: 11, borderRadius: 9, background: '#ff6b6b'}} />
      <span style={{width: 11, height: 11, borderRadius: 9, background: '#f0bc57'}} />
      <span style={{width: 11, height: 11, borderRadius: 9, background: '#54ca77'}} />
      <div style={{marginLeft: 12, flex: 1, height: 29, display: 'flex', alignItems: 'center', justifyContent: 'center', borderRadius: 8, background: incognito ? '#1c1a24' : '#151c24', border: `1px solid ${incognito ? '#474154' : '#2d3a47'}`}}>{title}</div>
      {incognito ? <span style={{fontSize: 18}}>♙</span> : <span style={{color: colors.blue, fontSize: 17}}>◉</span>}
    </div>
    {children}
  </div>
);
