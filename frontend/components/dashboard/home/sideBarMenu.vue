<script setup lang="ts">
import {list} from "postcss";

const data = ref({
  Features: [
    {
      name: "جداول",
      path: ""
    },
    {
      name: "فرم ها",
      path: ""
    },
    {
      name: "تفویم",
      path: ""
    },
    {
      name: "نقشه",
      path: ""
    },
  ],
  pages: [
    {
      name: "ورود",
      path: "/auth/login"
    },
    {
      name: "ثبتنام",
      path: ""
    },
    {
      name: "فراموشی رمز عبور",
      path: "/auth/forgot-password"
    },
    {
      name: "مقالات",
      path: "/dashboard/articles"
    },
  ],
  UIElements: [
    {
      name: "Button",
      path: ""
    },
    {
      name: "Badges",
      path: ""
    },
    {
      name: "Tabs",
      path: ""
    },
    {
      name: "Cards",
      path: ""
    },
    {
      name: "Alerts",
      path: ""
    },
    {
      name: "Progress Bars",
      path: ""
    },
    {
      name: "Modals",
      path: ""
    },
    {
      name: "Switchs",
      path: ""
    },
    {
      name: "Grids",
      path: ""
    },
    {
      name: "FontAwesome",
      path: ""
    },
    {
      name: "Typography",
      path: ""
    },
  ]
})
onMounted(() => {
  const listMenu = document.querySelectorAll('a.js-arrow')
  const show = document.querySelectorAll('.navbar__sub-list.js-sub-list')
  for (let i = 0; i < listMenu.length; i++) {
    listMenu[i].addEventListener("click", () => {
      show[i - 1].classList.toggle("show")
    })
  }
})
</script>

<template>
  <nav class="navbar-sidebar2 navbar-sidebar3">
    <ul class="list-unstyled navbar__list">
      <li class="active has-sub">
        <nuxt-link class="js-arrow text-decoration-none" to="">
          <i class="fas fa-tachometer-alt"></i>داشبرد
        </nuxt-link>
      </li>
      <li>
        <nuxt-link class="text-decoration-none" to="">
          <i class="fas fa-chart-bar"></i>پیام ها
        </nuxt-link>
        <span class="inbox-num">3</span>
      </li>
      <li>
        <nuxt-link class="text-decoration-none" to="">
          <i class="fas fa-shopping-basket"></i>سبد خرید
        </nuxt-link>
      </li>
      <li class="has-sub">
        <nuxt-link class="js-arrow text-decoration-none" to="">
          <i class="fas fa-trophy"></i>ویژگی ها
          <span class="arrow">
            <i class="fas fa-angle-down"></i>
          </span>
        </nuxt-link>
        <ul class="list-unstyled navbar__sub-list js-sub-list">
          <li v-for="(item , index) in data.Features" :key="index">
            <nuxt-link class="text-decoration-none" :to="item.path">{{ item.name }}</nuxt-link>
          </li>
        </ul>
      </li>
      <li class="has-sub">
        <nuxt-link class="js-arrow text-decoration-none" to="">
          <i class="fas fa-copy"></i>صفحات
          <span class="arrow">
            <i class="fas fa-angle-down"></i>
          </span>
        </nuxt-link>
        <ul class="list-unstyled navbar__sub-list js-sub-list">
          <li v-for="(item,index) in data.pages" :key="index">
            <nuxt-link class="text-decoration-none" :to="item.path">{{ item.name }}</nuxt-link>
          </li>
        </ul>
      </li>
      <li class="has-sub">
        <nuxt-link class="js-arrow text-decoration-none" to="">
          <i class="fas fa-desktop"></i>عناصر نمایشی
          <span class="arrow">
            <i class="fas fa-angle-down"></i>
          </span>
        </nuxt-link>
        <ul class="list-unstyled navbar__sub-list js-sub-list">
          <li v-for="(item , index) in data.UIElements" :key="index">
            <nuxt-link class="text-decoration-none" :to="item.path">{{ item.name }}</nuxt-link>
          </li>
        </ul>
      </li>
    </ul>
  </nav>
</template>

<style scoped>
.show {
  display: block !important;
}
</style>