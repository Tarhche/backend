<script setup>
import img from '~/assets/images/bg-title-01.jpg'
const data = ref({
  date: "22 May, 2018",
  tasksName: "Tasks for John Doe",
  tasks: [
    {
      info: "Meeting about plan for Admin Template 2018",
      time: "10:00 AM",
      class:"au-task__item--danger"
    },
    {
      info: "Create new task for Dashboard",
      time: "11:00 AM",
      class:"au-task__item--warning"
    },
    {
      info: "Meeting about plan for Admin Template 2018",
      time: "02:00 PM",
      class:"au-task__item--primary"
    },
    {
      info: "Create new task for Dashboard",
      time: "03:30 AM",
      class:"au-task__item--success"
    },
    {
      info: "Meeting about plan for Admin Template 2018",
      time: "10:00 AM",
      class:"au-task__item--danger"
    },
    {
      info: "Create new task for Dashboard",
      time: "03:30 AM",
      class:"au-task__item--warning" //هرچه قدر که نتونه نمایش بده که نیاز به اسکرول باشه رو با کلاس js-load-item نمایش میده
    },
  ]
})
</script>

<template>
  <div class="au-card au-card--no-shadow au-card--no-pad m-b-40 au-card--border">
    <div class="au-card-title" :style="{background:`url(${img}) center/cover`}">
      <div class="bg-overlay bg-overlay--blue"></div>
      <h3>
        <i class="zmdi zmdi-account-calendar"></i>{{data.date}}</h3>
      <button class="au-btn-plus">
        <i class="zmdi zmdi-plus"></i>
      </button>
    </div>
    <div class="au-task js-list-load au-task--border">
      <div class="au-task__title">
        <p>{{data.tasksName}}</p>
      </div>
      <div class="au-task-list js-scrollbar3">
        <div class="au-task__item au-task__item--danger" v-for="(item,index) in data.tasks" :key="index">
          <div class="au-task__item-inner "  >
            <h5 class="task">
              <nuxt-link to="#">{{item.info}}</nuxt-link>
            </h5>
            <span class="time">{{item.time}}</span>
          </div>
        </div>
      </div>
      <div class="au-task__footer">
        <button class="au-btn au-btn-load js-load-btn">load more</button>
      </div>
    </div>
  </div>
</template>

<style scoped>

</style>