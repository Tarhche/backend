<script setup lang="ts">

const data =ref({
  Features:[
    {
      name:"Tables",
      path:""
    },
    {
      name:"Forms",
      path:""
    },
    {
      name:"Calendar",
      path:""
    },
    {
      name:"Maps",
      path:""
    },
  ],
  pages:[
    {
      name:"Login",
      path:""
    },
    {
      name:"Register",
      path:""
    },
    {
      name:"Forget Password",
      path:""
    },
  ],
  UIElements:[
    {
      name:"Button",
      path:""
    },
    {
      name:"Badges",
      path:""
    },
    {
      name:"Tabs",
      path:""
    },
    {
      name:"Cards",
      path:""
    },
    {
      name:"Alerts",
      path:""
    },
    {
      name:"Progress Bars",
      path:""
    },
    {
      name:"Modals",
      path:""
    },
    {
      name:"Switchs",
      path:""
    },
    {
      name:"Grids",
      path:""
    },
    {
      name:"FontAwesome",
      path:""
    },
    {
      name:"Typography",
      path:""
    },
  ]
})
</script>

<template>
  <div class="" dir="rtl">
    <div class="menu-sidebar__content js-scrollbar1">
      <nav class="navbar-sidebar">
        <ul class="list-unstyled navbar__list">
          <li class="has-sub">
            <a class="js-arrow" href="#">
              <i class=" ms-2 fas fa-tachometer-alt"></i>Dashboard</a>
          </li>
          <li>
            <nuxt-link to="chart.html">
              <i class=" ms-2 fas fa-chart-bar"></i>Charts</nuxt-link>
          </li>
          <li>
            <nuxt-link to="table.html">
              <i class=" ms-2 fas fa-table"></i>Tables</nuxt-link>
          </li>
          <li class="active">
            <nuxt-link to="form.html">
              <i class=" ms-2 far fa-check-square"></i>Forms</nuxt-link>
          </li>
          <li>
            <nuxt-link to="calendar.html">
              <i class=" ms-2 fas fa-calendar-alt"></i>Calendar</nuxt-link>
          </li>
          <li>
            <nuxt-link to="map.html">
              <i class=" ms-2 fas fa-map-marker-alt"></i>Maps</nuxt-link>
          </li>
          <li class="has-sub">
            <a class="js-arrow" href="#">
              <i class=" ms-2 fas fa-copy"></i>Pages</a>
            <ul class="list-unstyled navbar__sub-list js-sub-list ">
              <li v-for="(item , index) in data.pages" :key="index">
                <nuxt-link to="login.html">{{item.name}}}</nuxt-link>
              </li>
            </ul>
          </li>
          <li class="has-sub">
            <nuxt-link class="js-arrow" to="#">
              <i class=" ms-2 fas fa-desktop"></i>UI Elements</nuxt-link>
            <ul class="list-unstyled navbar__sub-list js-sub-list ">
              <li v-for="(item , index) in data.UIElements" :key="index">
                <nuxt-link :to="item.path">{{item.name}}</nuxt-link>
              </li>
            </ul>
          </li>
        </ul>
      </nav>
    </div>
  </div>
</template>

<style scoped>

</style>