import { FaPlusCircle, FaBan, FaEdit, FaCheck, FaTrash } from "react-icons/fa";
import { type ReactNode, useState } from "react";
import { fetchTasks, addTask, deleteTask, updateTask } from "./tasks";
import "./App.css";

function App() {
	return (
		<>
			<h1>TODO App Demo</h1>
			<div
				style={{
					display: "flex",
					gap: "0.8rem",
					flexDirection: "column",
				}}
			>
				<Menu />
				<TaskList />
			</div>
		</>
	);
}

function Menu() {
	return (
		<nav
			style={{
				display: "flex",
				justifyContent: "flex-end",
			}}
		>
			<AddButton />
		</nav>
	);
}

function IconButton({
	onClick,
	children,
}: { onClick?: () => void; children: ReactNode }) {
	return (
		<button
			type="button"
			onClick={onClick}
			style={{
				display: "inline-flex",
				gap: "0.5rem",
				alignItems: "center",
			}}
		>
			{children}
		</button>
	);
}

function AddButton() {
	const onClickAdd = () => {
		addTask({
			name: "New Task",
			description: "Description",
		});
	};

	return (
		<IconButton onClick={onClickAdd}>
			<FaPlusCircle />
			Add
		</IconButton>
	);
}

function TaskList() {
	const { tasks } = fetchTasks();

	if (tasks.length === 0) {
		return (
			<article
				style={{
					minHeight: "10rem",
					placeContent: "center",
				}}
			>
				<p
					style={{
						fontSize: "2.5rem",
						opacity: 0.5,
						verticalAlign: "top",
						display: "inline-flex",
						alignItems: "center",
						gap: "0.5rem",
					}}
				>
					<FaBan />
					<span>Empty</span>
				</p>
			</article>
		);
	}

	return (
		<>
			{tasks.map((task) => (
				<Task
					key={task.id}
					taskId={task.id}
					initialName={task.name}
					initialDescription={task.description}
					initialEditable={false}
					deleteTask={() => deleteTask(task.id)}
				/>
			))}
		</>
	);
}

function TaskMenu({
	editable,
	onClickEdit,
	onClickDelete,
	onClickDone,
}: {
	editable: boolean;
	onClickEdit: () => void;
	onClickDone: () => void;
	onClickDelete: () => void;
}) {
	return (
		<nav
			style={{
				display: "flex",
				justifyContent: "flex-end",
				gap: "0.5rem",
				margin: "1rem 0",
			}}
		>
			{editable ? (
				<IconButton onClick={onClickDone}>
					<FaCheck />
					Done
				</IconButton>
			) : (
				<IconButton onClick={onClickEdit}>
					<FaEdit />
					Edit
				</IconButton>
			)}
			<IconButton onClick={onClickDelete}>
				<FaTrash />
				Delete
			</IconButton>
		</nav>
	);
}

function Task({
	taskId,
	initialName,
	initialDescription,
	initialEditable,
	deleteTask,
}: {
	taskId: string;
	initialName: string;
	initialDescription: string;
	initialEditable: boolean;
	deleteTask: () => void;
}) {
	const [editable, setEditable] = useState(initialEditable);
	const [name, setName] = useState(initialName);
	const [description, setDescription] = useState(initialDescription);

	const onClickDone = async () => {
		setEditable(false);
		const task = await updateTask({
			id: taskId,
			name,
			description,
		});
		if (!task) return;
		setName(task.name);
		setDescription(task.description);
	};

	return (
		<article
			style={{
				textAlign: "left",
				padding: "0 1rem",
				backgroundColor: "#343434",
				minHeight: "19rem",
				placeContent: "center",
				borderRadius: "0.5rem",
			}}
		>
			<h2>Task #{taskId}</h2>
			<TaskName
				editable={editable}
				name={name}
				onChange={(value) => setName(value)}
			/>
			<h3>Description</h3>
			<TaskDescription
				editable={editable}
				description={description}
				onChange={(value) => setDescription(value)}
			/>
			<TaskMenu
				editable={editable}
				onClickEdit={() => setEditable(true)}
				onClickDone={onClickDone}
				onClickDelete={() => deleteTask()}
			/>
		</article>
	);
}

function TaskName({
	editable,
	name,
	onChange,
}: { editable: boolean; name: string; onChange: (value: string) => void }) {
	if (editable) {
		return (
			<input
				style={{ fontSize: "1rem", width: "22rem" }}
				type="text"
				value={name}
				onChange={(e) => onChange(e.target.value)}
			/>
		);
	}

	return <p>{name}</p>;
}

function TaskDescription({
	editable,
	description,
	onChange,
}: {
	editable: boolean;
	description: string;
	onChange: (value: string) => void;
}) {
	if (editable) {
		return (
			<textarea
				style={{ fontSize: "1rem" }}
				cols={43}
				rows={3}
				value={description}
				onChange={(e) => onChange(e.target.value)}
			/>
		);
	}

	return <p style={{ minHeight: "4rem" }}>{description}</p>;
}

export default App;
