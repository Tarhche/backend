// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: [
    '~/assets/scss/main.scss',
  ],
  features: {
    inlineStyles: false,
  },
  modules: [
    '@vueuse/nuxt',
    'dayjs-nuxt',
  ],
  runtimeConfig: {
    internalApiBaseUrl: '',
    public: {
      apiBaseUrl: '',
    }
  },
  dayjs: {
    locales: ['en', 'fa'],
    plugins: ['relativeTime', 'utc', 'timezone'],
    defaultLocale: 'fa',
  },  
  app:{
    head:{
      htmlAttrs:{
        dir: "rtl",
        lang: "fa",
      }
    }
  }
})
