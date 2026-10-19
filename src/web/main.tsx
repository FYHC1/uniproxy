import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { IconContext } from '@phosphor-icons/react';
import { App } from './App';
import { AppProvider } from './state';
import './styles.css';
import './poolStyles.css';

const root = document.getElementById('root');
if (!root) throw new Error('UniProxy root element is missing');
createRoot(root).render(<StrictMode><IconContext.Provider value={{'aria-hidden':true,focusable:false}}><AppProvider><App /></AppProvider></IconContext.Provider></StrictMode>);
