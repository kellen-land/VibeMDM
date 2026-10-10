import { useMemo } from 'react'
import CodeMirror, { type ReactCodeMirrorProps } from '@uiw/react-codemirror'
import { StreamLanguage } from '@codemirror/language'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { xml } from '@codemirror/legacy-modes/mode/xml'
import { EditorView } from '@codemirror/view'
import { useTheme } from '../theme/ThemeContext'

export type CodeLanguage = 'shell' | 'xml' | 'plain'

export interface CodeEditorProps extends Omit<ReactCodeMirrorProps, 'extensions' | 'theme'> {
  language?: CodeLanguage
}

const shellLang = StreamLanguage.define(shell)
const xmlLang = StreamLanguage.define(xml)
const baseStyle = EditorView.theme({
  '&': { fontSize: '13px' },
  '.cm-content': { fontFamily: 'var(--font-mono)' },
  '.cm-gutters': { fontFamily: 'var(--font-mono)' },
})

export default function CodeEditorImpl({ language = 'shell', height = '320px', ...rest }: CodeEditorProps) {
  const { theme } = useTheme()
  const dark = theme === 'dark'
  const extensions = useMemo(() => {
    const ext = [baseStyle, EditorView.lineWrapping]
    if (language === 'shell') ext.push(shellLang)
    if (language === 'xml') ext.push(xmlLang)
    return ext
  }, [language])
  return <CodeMirror theme={dark ? 'dark' : 'light'} extensions={extensions} height={height} basicSetup={{ foldGutter: false, highlightActiveLine: true }} {...rest} />
}
