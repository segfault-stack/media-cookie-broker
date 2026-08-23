import type {ReactNode} from 'react';
import {colors, sans} from '../theme';
import {BrokerNode} from './BrokerNode';
import {BrowserWindow} from './BrowserWindow';
import {ExtensionPopup} from './ExtensionPopup';
import {TerminalCard} from './TerminalCard';

export const Background: React.FC<{children: ReactNode}> = ({children}) => <div style={{position: 'absolute', inset: 0, overflow: 'hidden', background: 'radial-gradient(circle at 75% 30%,#16283a 0,#0d141c 36%,#090d12 78%)', fontFamily: sans, color: colors.text}}>
  <div style={{position: 'absolute', inset: 0, opacity: 0.16, backgroundImage: 'linear-gradient(#7790a51a 1px,transparent 1px),linear-gradient(90deg,#7790a51a 1px,transparent 1px)', backgroundSize: '42px 42px'}} />
  <div style={{position: 'absolute', left: 430, right: 430, top: 365, height: 1, background: 'linear-gradient(90deg,transparent,#667d9888,transparent)'}} />
  {children}
</div>;

export const DesktopStage: React.FC<{consumer: 'healthy17' | 'failure' | 'healthy18'; broker: 'healthy17' | 'attention17' | 'healthy18'; extension: 'healthy17' | 'attention17' | 'healthy18'; buttonPressed?: boolean; failureRevealAt?: number; children?: ReactNode}> = ({consumer, broker, extension, buttonPressed, failureRevealAt, children}) => <Background>
  <div style={{position: 'absolute', left: 50, top: 164}}><TerminalCard state={consumer} revealFailureAt={failureRevealAt} /></div>
  <div style={{position: 'absolute', left: 565, top: 274, zIndex: 10}}><BrokerNode state={broker} /></div>
  <div style={{position: 'absolute', left: 775, top: 119, width: 455, height: 500}}><BrowserWindow><ExtensionPopup state={extension} buttonPressed={buttonPressed} /></BrowserWindow></div>
  <div style={{position: 'absolute', left: 214, top: 104, fontSize: 12, letterSpacing: 1.7, fontWeight: 800, color: colors.faint}}>REMOTE CONSUMER</div>
  <div style={{position: 'absolute', left: 936, top: 79, fontSize: 12, letterSpacing: 1.7, fontWeight: 800, color: colors.faint}}>YOUR CHROMIUM</div>
  {children}
</Background>;
