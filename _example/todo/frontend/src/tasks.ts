import useSWR, { mutate } from "swr";
import { newClient, isErrorResponse } from "./client";

interface task {
	id: string;
	name: string;
	description: string;
}

const client = newClient();

export const useTasks = () => {};

export const fetchTasks = () => {
	const { data, error } = useSWR("/api/tasks", async () => {
		const data = await client.get("/api/tasks", {});
		if (isErrorResponse(data)) {
			throw new Error(data.error.message);
		}
		if (!data.tasks) {
			return [];
		}

		return data.tasks.map((task) => {
			return {
				id: task.id,
				name: task.name,
				description: task.description,
			};
		});
	});
	if (error) {
		throw error;
	}

	return { tasks: data || [] };
};

export const addTask = (task: {
	name: string;
	description: string;
}) => {
	mutate(
		"/api/tasks",
		async () => {
			const data = await client.post("/api/tasks", {
				data: {
					task: {
						name: task.name,
						description: task.description,
					},
				},
			});
			if (isErrorResponse(data)) {
				throw new Error(data.error.message);
			}
			return {
				id: data.task.id,
				name: data.task.name,
				description: data.task.description,
			};
		},
		{
			populateCache: (task: task, tasks: task[] | undefined) => {
				if (tasks === undefined) {
					return [task];
				}
				return [task, ...tasks];
			},
			revalidate: false,
		},
	);
};

export const deleteTask = (taskId: string) => {
	mutate(
		"/api/tasks",
		async () => {
			const data = await client.delete("/api/tasks/{id}", {
				pathArgs: { id: taskId },
			});
			if (isErrorResponse(data)) {
				throw new Error(data.error.message);
			}
			return data || { status: "error" };
		},
		{
			populateCache: (
				result: { status: string },
				tasks: task[] | undefined,
			) => {
				if (result.status !== "ok") {
					return tasks || [];
				}
				return tasks?.filter((task) => task.id !== taskId) || [];
			},
			revalidate: false,
		},
	);
};

export const updateTask = async (task: task) => {
	return await mutate(
		"/api/tasks",
		async () => {
			const data = await client.put("/api/tasks/{id}", {
				pathArgs: { id: task.id },
				data: {
					task: {
						name: task.name,
						description: task.description,
						status: "done",
					},
				},
			});
			if (isErrorResponse(data)) {
				throw new Error(data.error.message);
			}
			return {
				id: data.task.id,
				name: data.task.name,
				description: data.task.description,
			};
		},
		{
			populateCache: (task: task, tasks: task[] | undefined) => {
				if (tasks === undefined) {
					return [task];
				}
				const newTasks = tasks.map((t) => {
					if (t.id === task.id) {
						return task;
					}
					return t;
				});
				return newTasks;
			},
			revalidate: true,
		},
	);
};
