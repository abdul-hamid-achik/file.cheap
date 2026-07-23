import DefaultTheme from 'vitepress/theme-without-fonts'
import { h } from 'vue'
import type { Theme } from 'vitepress'
import './styles/custom.css'

import DocBreadcrumbs from './components/DocBreadcrumbs.vue'
import HomePage from './components/HomePage.vue'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'doc-before': () => h(DocBreadcrumbs),
    }),
  enhanceApp({ app }) {
    app.component('HomePage', HomePage)
  },
} satisfies Theme
