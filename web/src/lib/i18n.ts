import i18n from "i18next"
import { initReactI18next } from "react-i18next"
import LanguageDetector from "i18next-browser-languagedetector"
import zhCN from "@/locales/zh-CN.json"
import enUS from "@/locales/en-US.json"

export const LANGUAGE_STORAGE_KEY = "mmonitor:lang"

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      "zh-CN": { translation: zhCN },
      "en-US": { translation: enUS },
    },
    fallbackLng: "en-US",
    supportedLngs: ["zh-CN", "en-US"],
    load: "currentOnly",
    detection: {
      order: ["localStorage", "navigator"],
      lookupLocalStorage: LANGUAGE_STORAGE_KEY,
      caches: ["localStorage"],
      convertDetectedLanguage: (lng: string) => {
        const lower = lng.toLowerCase()
        if (lower.startsWith("zh")) return "zh-CN"
        return "en-US"
      },
    },
    interpolation: {
      escapeValue: false,
    },
  })

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng
})
document.documentElement.lang = i18n.language || "en-US"

export default i18n
